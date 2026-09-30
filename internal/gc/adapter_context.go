package gc

import (
	"fmt"
	"go-backend-ssa/internal/cmd/bio"
	"go-backend-ssa/internal/cmd/objabi"
	"go-backend-ssa/internal/go/buildcfg"
	"go-backend-ssa/internal/ir"
	"go-backend-ssa/internal/objw"
	"go-backend-ssa/internal/pkginit"
	"io"
	"strings"
)

// Context binds the compiler's sequential object-finalization operations.
// Later driver stages share the same writer and compiler graph.
type Context struct {
	header           *objabi.Context
	archiveModes     ObjectMode
	archiveFailed    bool
	output           *archiveOutput
	outputMessages   []string
	Writer           *objw.Context
	Package          *pkginit.Context
	functionDataDone bool
}

func NewContext(writer *objw.Context) *Context {
	if writer == nil || writer.Link == nil || writer.Types == nil {
		panic("compiler finalization requires an object writer")
	}
	return &Context{Writer: writer}
}

// AddGCLocals runs the native sequential registration of all function auxiliary
// data after Flush and before symbol numbering. Call once per compilation.
func (c *Context) AddGCLocals() {
	if c == nil || c.Writer == nil || c.functionDataDone {
		panic("function data finalization requires an unconsumed compiler context")
	}
	c.functionDataDone = true
	c.addGCLocals()
}

// BindPackage shares the package initialization and reflection graph used by
// global declaration finalization. It must be bound before DumpGlobal.
func (c *Context) BindPackage(p *pkginit.Context) {
	if c == nil || c.Writer == nil || p == nil || p.Noder == nil || p.Noder.Typecheck == nil || p.Noder.Typecheck.IR == nil || p.Noder.ReflectData == nil || p.Noder.ReflectData.Typecheck != p.Noder.Typecheck || p.Noder.ReflectData.Runtime == nil || p.Noder.Typecheck.IR.Types != c.Writer.Types || p.Noder.Typecheck.IR.Base.Ctxt != c.Writer.Link || p.Noder.ReflectData.Runtime.Writer != c.Writer || c.Package != nil {
		panic("global finalization requires the same unbound package graph")
	}
	c.Package = p
}
func (c *Context) checkGlobal(n *ir.Name) {
	if c == nil || c.Package == nil || c.functionDataDone || n == nil || !c.Package.Noder.Typecheck.IR.OwnsNode(n) {
		panic("global belongs to another or finalized compilation")
	}
}

// DumpGlobal emits the native data definition and DWARF for a global name.
func (c *Context) DumpGlobal(n *ir.Name) { c.checkGlobal(n); c.dumpGlobal(n) }

// DumpGlobalConst emits the native typed integer constant DWARF record.
func (c *Context) DumpGlobalConst(n *ir.Name) { c.checkGlobal(n); c.dumpGlobalConst(n) }
func (c *Context) fatalf(f string, a ...any)  { panic(fmt.Errorf(f, a...)) }

// ObjectConfig carries explicit target features and compiler version for the
// native compatibility header. Experiments come from the bound Universe.
type ObjectConfig = buildcfg.Context
type ObjectMode int

const (
	CompilerObject ObjectMode = modeCompilerObj
	LinkerObject   ObjectMode = modeLinkerObj
	CombinedObject            = CompilerObject | LinkerObject
)

// SetObjectHeader freezes target/header inputs after compiler data finalization.
func (c *Context) SetObjectHeader(config ObjectConfig) {
	if c == nil || c.Writer == nil || !c.functionDataDone || c.header != nil || config.GOOS != c.Writer.Link.TargetOS || config.GOARCH != c.Writer.Link.Arch.Name || config.Version == "" {
		panic("object header requires finalized, matching target inputs")
	}
	exp := c.Writer.Types.Experiment
	c.header = &objabi.Context{ObjectConfig: &config, Experiment: exp}
}

// WriteArchive emits the native compiler/linker archive members selected by
// mode. It numbers the finalized graph if needed. Compiler and linker outputs
// may be split, with the compiler member written first. Each member is single-
// use even after an I/O failure. The caller owns and closes output.
func (c *Context) WriteArchive(output io.WriteSeeker, mode ObjectMode) (err error) {
	if c == nil || c.Writer == nil || c.Package == nil || c.header == nil || !c.functionDataDone || output == nil || mode == 0 || mode&^CombinedObject != 0 || c.archiveModes&mode != 0 || c.archiveFailed || c.output != nil {
		return fmt.Errorf("archive requires finalized, configured and unconsumed compiler state")
	}
	link := c.Writer.Link
	if link.Errors != 0 || c.Package.Noder.Diagnostics.HasErrors() || link.ObjectWritten() {
		return fmt.Errorf("archive cannot follow compiler errors or an existing linker object")
	}
	if mode&CompilerObject != 0 && !c.Package.Noder.ExportAvailable() {
		return fmt.Errorf("archive compiler member requires an unused local export")
	}
	if mode == LinkerObject && c.archiveModes&CompilerObject == 0 {
		return fmt.Errorf("write compiler exports before a separate linker archive")
	}
	if !link.ObjectNumbered() {
		c.Writer.NumberSymbols()
	}
	c.archiveModes |= mode
	c.output = &archiveOutput{WriteSeeker: output}
	c.outputMessages = nil
	oldDiag := link.DiagFunc
	link.DiagFunc = func(f string, a ...any) {
		c.outputMessages = append(c.outputMessages, fmt.Sprintf(f, a...))
		if oldDiag != nil {
			oldDiag(f, a...)
		}
	}
	defer func() {
		link.DiagFunc = oldDiag
		c.output = nil
		if p := recover(); p != nil {
			c.archiveFailed = true
			if e, ok := p.(*bio.OutputError); ok {
				err = e
			} else {
				panic(p)
			}
		}
		if err != nil {
			c.archiveFailed = true
		}
	}()
	c.dumpobj1(c.Package.Noder.Flag.LowerO, int(mode))
	if len(c.outputMessages) != 0 {
		return fmt.Errorf("archive writing: %s", strings.Join(c.outputMessages, "; "))
	}
	return nil
}

type archiveOutput struct {
	io.WriteSeeker
	failure error
}

func (s *archiveOutput) Write(p []byte) (int, error) {
	n, e := s.WriteSeeker.Write(p)
	if s.failure == nil {
		s.failure = e
	}
	return n, e
}
func (s *archiveOutput) Seek(n int64, w int) (int64, error) {
	off, e := s.WriteSeeker.Seek(n, w)
	if s.failure == nil {
		s.failure = e
	}
	return off, e
}
func (c *Context) createOutput(string) (*bio.Writer, error) {
	if c.output == nil {
		return nil, fmt.Errorf("archive output stream is not configured")
	}
	return bio.NewWriter(c.output), nil
}
func (c *Context) tracePrintf(f string, a ...any) (int, error) {
	message := fmt.Sprintf(f, a...)
	c.outputMessages = append(c.outputMessages, message)
	if c.Writer.Link.TraceWriter != nil {
		return io.WriteString(c.Writer.Link.TraceWriter, message)
	}
	return len(message), nil
}
func (c *Context) flushErrors() {
	for _, d := range c.Package.Noder.Diagnostics.Diagnostics() {
		c.outputMessages = append(c.outputMessages, d.String())
	}
	if f, ok := c.Writer.Link.TraceWriter.(interface{ Flush() error }); ok {
		_ = f.Flush()
	}
}
func (c *Context) errorExit() {
	var cause error
	if c.output != nil {
		cause = c.output.failure
	}
	panic(&bio.OutputError{Message: strings.Join(c.outputMessages, "; "), Cause: cause})
}

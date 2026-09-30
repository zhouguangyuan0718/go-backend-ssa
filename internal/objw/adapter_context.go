package objw

import (
	"fmt"
	"go-backend-ssa/diagnostic"
	"go-backend-ssa/internal/cmd/bio"
	"go-backend-ssa/internal/cmd/obj"
	"go-backend-ssa/internal/ir"
	"go-backend-ssa/internal/types"
	"io"
	"strings"
)

// Context binds the native writer to an existing compiler Universe and Link.
// Callers serialize mutation of a Context and its symbols.
type Context struct {
	IR              *ir.Context
	sharedProgArray *[10000]obj.Prog
	Types           *types.Universe
	Link            *obj.Link
	ptrSize         int
}

func NewContext(u *types.Universe, link *obj.Link, goos string) *Context {
	if u == nil || link == nil || link.Arch == nil || link.Arch.Arch == nil || goos == "" || link.Arch.ByteOrder == nil {
		panic("object writer requires an explicit target and byte order")
	}
	if u.PosTable != &link.PosTable || link.Arch.PtrSize != u.PointerSize() || link.Arch.RegSize != u.RegisterSize() {
		panic("object writer target/position mismatch")
	}
	if link.TargetOS != "" && link.TargetOS != goos {
		panic("object writer cannot change the target OS")
	}
	u.BindLink(link)
	link.TargetOS = goos
	c := newContext()
	c.Types = u
	c.Link = link
	c.ptrSize = u.PointerSize()
	return c
}
func (c *Context) fatalf(format string, args ...any) { panic(fmt.Errorf(format, args...)) }

func (c *Context) checkNewProgs(fn *ir.Func, worker int) {
	g := ir.ContextOf(fn)
	if g == nil || g.Types != c.Types || g.Base.Ctxt != c.Link || worker < 0 || worker >= g.Base.Flag.LowerC {
		panic("instruction writer requires an owned function and valid worker")
	}
	if c.IR != nil && c.IR != g {
		panic("instruction writer graph mismatch")
	}
	if c.IR == nil {
		c.IR = g
	}
}
func (c *Context) warn(format string, args ...any) {
	c.IR.Diagnostics.Add(diagnostic.Warning, "", c.Link.OutermostPos(c.Types.Pos), fmt.Sprintf(format, args...))
}

// OwnsProgs reports whether pp is an active instruction buffer of this writer.
func (c *Context) OwnsProgs(pp *Progs) bool {
	return c != nil && pp != nil && pp.compilation == c && pp.CurFunc != nil && pp.Text != nil && pp.Text.Ctxt == c.Link
}

func flushContext(pp *Progs) *Context {
	if pp == nil || pp.compilation == nil || !pp.compilation.OwnsProgs(pp) {
		panic("flush requires an active instruction buffer")
	}
	c := pp.compilation
	if ir.ContextOf(pp.CurFunc) != c.IR || pp.Text.From.Sym != pp.CurFunc.LSym {
		panic("flush function owner mismatch")
	}
	if c.Link.DWARF == nil || c.Link.DebugInfo == nil {
		panic("instruction flush requires compiler DWARF callbacks")
	}
	return c
}

// WriteObject numbers symbols if needed and writes the native Go object payload. Finish
// all functions, static/type data and DWARF fixups first. Each Link can be
// serialized once; failure also consumes this stage. The caller owns output,
// including any partial bytes after failure, and supplies file-backed data via
// Link.OpenObjectInput. This is the binary payload, without an archive/export
// header. Operations within the compiler instance must be serialized.
func (c *Context) WriteObject(output io.WriteSeeker) (err error) {
	if c == nil || c.Link == nil || c.Types == nil || output == nil {
		return fmt.Errorf("object writing requires a compiler context and output stream")
	}
	if c.Link.Errors != 0 {
		return fmt.Errorf("cannot write object after %d diagnostics", c.Link.Errors)
	}
	b := bio.NewWriter(output)
	link := c.Link
	oldDiag := link.DiagFunc
	var messages []string
	link.DiagFunc = func(f string, a ...any) {
		messages = append(messages, fmt.Sprintf(f, a...))
		if oldDiag != nil {
			oldDiag(f, a...)
		}
	}
	defer func() {
		link.DiagFunc = oldDiag
		if p := recover(); p != nil {
			if e, ok := p.(*bio.OutputError); ok {
				err = e
			} else {
				panic(p)
			}
		}
	}()
	if !link.ObjectNumbered() {
		c.NumberSymbols()
	}
	obj.WriteObjFile(link, b)
	if e := b.Flush(); e != nil {
		return e
	}
	if len(messages) != 0 {
		return fmt.Errorf("object writing: %s", strings.Join(messages, "; "))
	}
	return nil
}

// NumberSymbols assigns native object indices before finalized package export.
// The complete object graph must be ready, and numbering is single-use.
func (c *Context) NumberSymbols() {
	if c == nil || c.Link == nil || c.Types == nil {
		panic("symbol numbering requires a compiler writer")
	}
	c.Link.NumberObjectSymbols(c.Types.Experiment.RegabiWrappers)
}

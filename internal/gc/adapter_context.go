package gc

import (
	"fmt"
	"go-backend-ssa/diagnostic"
	"go-backend-ssa/internal/bloop"
	"go-backend-ssa/internal/cmd/bio"
	"go-backend-ssa/internal/cmd/obj"
	"go-backend-ssa/internal/cmd/objabi"
	"go-backend-ssa/internal/cmd/src"
	"go-backend-ssa/internal/deadlocals"
	"go-backend-ssa/internal/go/buildcfg"
	"go-backend-ssa/internal/ir"
	"go-backend-ssa/internal/loopvar"
	"go-backend-ssa/internal/objw"
	"go-backend-ssa/internal/pgoir"
	"go-backend-ssa/internal/pkginit"
	"go-backend-ssa/internal/slice"
	"go-backend-ssa/internal/ssagen"
	"go-backend-ssa/internal/walk"
	"io"
	"math/rand"
	"strings"
	"sync"
)

// Context binds native function preparation, backend scheduling and object
// finalization to the same compiler graph. Public stage calls are serialized;
// CompileQueued runs the configured backend workers and joins them before return.
type Context struct {
	bloop                                            *bloop.Context
	loopvar                                          *loopvar.Context
	deadlocals                                       *deadlocals.Context
	slice                                            *slice.Context
	frontendStarted, frontendRunning, frontendFailed bool
	finishStarted, finishRunning, finishFailed       bool
	Backend                                          *ssagen.Context
	Lowering                                         *walk.Context
	compilequeue                                     []*ir.Func
	enqueued                                         map[*ir.Func]bool
	workerFailureMu                                  sync.Mutex
	workerFailure                                    any
	// RaceEnabled selects the native randomized scheduling used by race builds.
	// Randomness is owned by this compilation and cannot affect another instance.
	RaceEnabled bool
	random      *rand.Rand

	header                           *objabi.Context
	assemblyStarted, assemblyRunning bool
	archiveModes                     ObjectMode
	archiveFailed                    bool
	output                           *archiveOutput
	outputMessages                   []string
	Writer                           *objw.Context
	Package                          *pkginit.Context
	functionDataDone                 bool
	dataFailed                       bool
	declarationsStarted              bool
	declarationsRunning              bool
	declarationsFailed               bool
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
	if c == nil || c.Writer == nil || c.functionDataDone || c.finishStarted || c.frontendRunning || c.frontendFailed || c.declarationsRunning || c.declarationsFailed || c.workerFailure != nil || len(c.compilequeue) != 0 {
		panic("function data finalization requires an unconsumed compiler context")
	}
	c.functionDataDone = true
	c.addGCLocals()
}

// DumpData runs native GC, plugin, embed, map-zero and function-data finalization.
// Call after the package type/function fixed point and DWARF finalization.
// A started data stage is consumed even if an input provider fails.
func (c *Context) DumpData() {
	if c == nil || c.Package == nil || c.functionDataDone || c.finishFailed || c.finishStarted && !c.finishRunning || c.frontendRunning || c.frontendFailed || c.declarationsRunning || c.declarationsFailed || c.workerFailure != nil || len(c.compilequeue) != 0 || c.Writer.Link.ObjectNumbered() || c.Package.Noder.StaticInit == nil || c.Package.Noder.ReflectData.StaticData == nil || c.Package.Noder.ReflectData.StaticData.Writer != c.Writer || c.Package.Noder.ReflectData.StaticData.IR != c.Package.Noder.Typecheck.IR {
		panic("package data requires an unconsumed matching compiler graph")
	}
	c.functionDataDone = true
	c.dataFailed = true
	c.dumpdata()
	c.dataFailed = c.errors() != 0
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
	if c == nil || c.Package == nil || c.functionDataDone || c.finishStarted || c.frontendRunning || c.frontendFailed || c.declarationsStarted && !c.declarationsRunning || c.workerFailure != nil || n == nil || !c.Package.Noder.Typecheck.IR.OwnsNode(n) {
		panic("global belongs to another or finalized compilation")
	}
}

// DumpGlobal emits the native data definition and DWARF for a global name.
func (c *Context) DumpGlobal(n *ir.Name) { c.checkGlobal(n); c.dumpGlobal(n) }

// DumpGlobalConst emits the native typed integer constant DWARF record.
func (c *Context) DumpGlobalConst(n *ir.Name) { c.checkGlobal(n); c.dumpGlobalConst(n) }
func (c *Context) fatalf(f string, a ...any) {
	if c.assemblyRunning && len(a) == 1 {
		if cause, ok := a[0].(error); ok {
			panic(&bio.OutputError{Message: fmt.Sprintf(f, a...), Cause: cause})
		}
	}
	panic(fmt.Errorf(f, a...))
}

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
	if c == nil || c.Writer == nil || !c.functionDataDone || c.finishRunning || c.finishFailed || c.dataFailed || c.header != nil || config.GOOS != c.Writer.Link.TargetOS || config.GOARCH != c.Writer.Link.Arch.Name || config.Version == "" {
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
	if c == nil || c.Writer == nil || c.Package == nil || c.header == nil || !c.functionDataDone || output == nil || mode == 0 || mode&^CombinedObject != 0 || c.archiveModes&mode != 0 || c.archiveFailed || c.finishRunning || c.finishFailed || c.output != nil {
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

// BindBackend attaches the initialized backend and frontend lowering graph.
func (c *Context) BindBackend(backend *ssagen.Context, lowering *walk.Context) {
	if c == nil || c.Package == nil || backend == nil || lowering == nil || c.Backend != nil || c.functionDataDone || backend.Typecheck != c.Package.Noder.Typecheck || backend.SSA == nil || backend.SSAConfig() == nil || backend.WorkerCount() != backend.Typecheck.IR.Base.Flag.LowerC || backend.SSA.ReflectData != c.Package.Noder.ReflectData || lowering.Noder != c.Package.Noder || lowering.Escape == nil || lowering.Escape.Typecheck != c.Package.Noder.Typecheck || backend.Inline != c.Package.Noder.Inline {
		panic("compiler queue requires the same initialized backend and package graph")
	}
	c.Writer.IR = c.Package.Noder.Typecheck.IR
	c.Backend = backend
	c.Lowering = lowering
	c.enqueued = make(map[*ir.Func]bool)
	c.random = rand.New(rand.NewSource(1))
	c.bloop = bloop.NewContext(c.Package.Noder.Typecheck)
	c.loopvar = loopvar.NewContext(c.Package.Noder.Typecheck, c.Package.Noder.Inline.Log)
	c.deadlocals = deadlocals.NewContext(c.Package.Noder.Typecheck.IR)
	c.slice = slice.NewContext(lowering.Escape, c.Package.Noder.ReflectData)
}

// EnqueueFunc preserves native handling of closures, bodyless declarations and
// sequential frontend preparation. CompileQueued must finish before finalization.
func (c *Context) EnqueueFunc(fn *ir.Func, symABIs *ssagen.SymABIs) {
	if c == nil || c.Backend == nil || c.functionDataDone || c.finishStarted || c.frontendRunning || c.frontendFailed || c.declarationsStarted && !c.declarationsRunning || c.workerFailure != nil || fn == nil || !c.Package.Noder.Typecheck.IR.OwnsNode(fn) || !c.Backend.OwnsSymABIs(symABIs) || c.enqueued[fn] || c.Package.Noder.Typecheck.IR.CurFunc != nil || c.Writer.Link.ObjectNumbered() || (fn.LSym != nil && !fn.IsClosure()) {
		panic("compiler queue requires an unconsumed owned function and ABI input")
	}
	c.enqueued[fn] = true
	c.enqueueFunc(fn, symABIs)
}

// CompileQueued executes the native worker queue, including nested closures.
func (c *Context) CompileQueued(profile *pgoir.Profile) {
	if c == nil || c.Backend == nil || c.functionDataDone || c.finishStarted || c.frontendRunning || c.frontendFailed || c.declarationsStarted && !c.declarationsRunning || c.workerFailure != nil || c.Package.Noder.Typecheck.IR.CurFunc != nil {
		panic("compile queue is not ready")
	}
	if c.Backend.WorkerCount() != c.Package.Noder.Typecheck.IR.Base.Flag.LowerC || c.Backend.WorkerCount() < 1 || c.errors() != 0 || (profile != nil && !c.Backend.Inline.PGO.OwnsProfile(profile)) {
		panic("compiler queue has invalid workers, profile or diagnostics")
	}
	c.compileFunctions(profile)
	if c.workerFailure != nil {
		panic(c.workerFailure)
	}
}
func (c *Context) QueuedFunctions() int    { return len(c.compilequeue) }
func (c *Context) permutation(n int) []int { return c.random.Perm(n) }
func (c *Context) fatalfAt(pos src.XPos, f string, a ...any) {
	panic(fmt.Errorf("%s: %s", c.Package.Noder.Typecheck.IR.Base.FmtPos(pos), fmt.Sprintf(f, a...)))
}
func (c *Context) errors() int {
	n := 0
	g := c.Package.Noder.Typecheck.IR
	for _, ds := range [][]diagnostic.Diagnostic{g.Diagnostics.Diagnostics(), g.Types.Diagnostics.Diagnostics()} {
		for _, d := range ds {
			if d.Severity == diagnostic.Error {
				n++
			}
		}
	}
	return n
}

// compileWorker transfers backend panics to the calling goroutine after the
// native queue has joined all workers and restored its parallel-phase flags.
// Any such failure consumes the compiler; partial output cannot be finalized.
func (c *Context) compileWorker(fn *ir.Func, worker int, profile *pgoir.Profile) {
	defer func() {
		if failure := recover(); failure != nil {
			c.workerFailureMu.Lock()
			if c.workerFailure == nil {
				c.workerFailure = failure
			}
			c.workerFailureMu.Unlock()
		}
	}()
	c.Backend.Compile(fn, worker, profile)
}

// CompileDeclarations runs the native package fixed point after frontend passes,
// generating runtime types, globals and functions until no more work appears.
// The native final DWARF pass participates in that same fixed point. Call once
// on a fresh bound backend; do not pre-enqueue functions or emit globals.
func (c *Context) CompileDeclarations(symABIs *ssagen.SymABIs, profile *pgoir.Profile) {
	if c == nil || c.Backend == nil || c.declarationsStarted || c.finishStarted || c.frontendRunning || c.frontendFailed || c.functionDataDone || c.workerFailure != nil || len(c.enqueued) != 0 || len(c.compilequeue) != 0 || c.Writer.Link.ObjectNumbered() || c.Package.Noder.Typecheck.IR.CurFunc != nil || !c.Backend.OwnsSymABIs(symABIs) || c.Backend.WorkerCount() != c.Package.Noder.Typecheck.IR.Base.Flag.LowerC || c.Backend.WorkerCount() < 1 || c.errors() != 0 || profile != nil && !c.Backend.Inline.PGO.OwnsProfile(profile) {
		panic("declaration compilation requires a fresh initialized backend and owned ABI/profile")
	}
	c.declarationsStarted, c.declarationsRunning = true, true
	completed := false
	defer func() { c.declarationsRunning = false; c.declarationsFailed = !completed }()
	c.compileDeclarations(symABIs, profile)
	completed = c.errors() == 0
}

// PrepareFrontend preserves Main's native pass order from the first IR report
// through inlining, wrappers, capture, initialization, escape and slice analysis.
// Call once after loading local IR and applying coverage fixups, before compiling
// declarations. A started pass consumes this stage even if a provider panics.
func (c *Context) PrepareFrontend(symABIs *ssagen.SymABIs, profile *pgoir.Profile) {
	if c == nil || c.Backend == nil || c.frontendStarted || c.declarationsStarted || c.functionDataDone || c.workerFailure != nil || len(c.enqueued) != 0 || len(c.compilequeue) != 0 || c.Writer.Link.ObjectNumbered() || c.Package.Noder.Typecheck.IR.CurFunc != nil || !c.Backend.OwnsSymABIs(symABIs) || c.errors() != 0 || profile != nil && !c.Backend.Inline.PGO.OwnsProfile(profile) {
		panic("frontend preparation requires a fresh compiler graph and owned ABI/profile")
	}
	n := c.Package.Noder
	if c.Backend.Typecheck != n.Typecheck || c.Backend.Inline != n.Inline || c.Lowering.Noder != n || c.Lowering.Escape.Typecheck != n.Typecheck || n.ReflectData.Typecheck != n.Typecheck || n.Interleaved.Inline != n.Inline || n.StaticInit.Typecheck != n.Typecheck || n.StaticInit.ReflectData != n.ReflectData || c.bloop.Typecheck != n.Typecheck || c.loopvar.Typecheck != n.Typecheck || c.loopvar.Log != n.Inline.Log || c.deadlocals.IR != n.Typecheck.IR || c.slice.Escape != c.Lowering.Escape || c.slice.ReflectData != n.ReflectData {
		panic("frontend preparation requires matching compiler dependencies")
	}
	for _, fn := range n.Typecheck.Target.Funcs {
		if fn == nil || !n.Typecheck.IR.OwnsNode(fn) {
			panic("frontend function belongs to another compilation")
		}
	}
	for _, v := range n.Typecheck.Target.Externs {
		if v == nil || !n.Typecheck.IR.OwnsNode(v) {
			panic("frontend declaration belongs to another compilation")
		}
	}
	for _, fn := range n.Typecheck.Target.Inits {
		if fn == nil || !n.Typecheck.IR.OwnsNode(fn) {
			panic("frontend initializer belongs to another compilation")
		}
	}
	for _, pkg := range n.Typecheck.Target.Imports {
		if pkg == nil || pkg.Universe() != n.Typecheck.IR.Types {
			panic("frontend import belongs to another compilation")
		}
		if sym := pkg.Syms[".inittask"]; sym != nil {
			if task, ok := sym.Def.(*ir.Name); ok && !n.Typecheck.IR.OwnsNode(task) {
				panic("frontend imported initializer belongs to another compilation")
			}
		}
	}

	c.frontendStarted, c.frontendRunning = true, true
	completed := false
	defer func() { c.frontendRunning = false; c.frontendFailed = !completed }()
	c.prepareFrontend(symABIs, profile)
	completed = c.errors() == 0
}

// FinishPackage preserves Main's post-compilation write-barrier checks, global
// map keep relocations, data finalization and symbol numbering. Call once after
// PrepareFrontend and CompileDeclarations. Archive output remains caller-owned.
func (c *Context) FinishPackage() {
	if c == nil || c.Backend == nil || !c.frontendStarted || c.frontendRunning || c.frontendFailed || !c.declarationsStarted || c.declarationsRunning || c.declarationsFailed || c.finishStarted || c.functionDataDone || c.dataFailed || c.workerFailure != nil || len(c.compilequeue) != 0 || c.Writer.Link.ObjectNumbered() || c.Package.Noder.Typecheck.IR.CurFunc != nil || c.errors() != 0 {
		panic("package finalization requires completed frontend and declaration stages")
	}
	c.finishStarted, c.finishRunning = true, true
	completed := false
	defer func() { c.finishRunning = false; c.finishFailed = !completed }()
	c.finishPackage()
	completed = c.errors() == 0 && !c.dataFailed && c.Writer.Link.ObjectNumbered()
}
func (c *Context) numberSymbols(link *obj.Link) {
	if link != c.Writer.Link || !c.finishRunning {
		panic("symbol numbering requires the active package writer")
	}
	c.Writer.NumberSymbols()
}

// CompilePackage compiles loaded and coverage-adjusted local IR through native
// frontend passes, declaration iteration and final symbol numbering. The caller
// configures the object header and writes archives after this method returns.
func (c *Context) CompilePackage(symABIs *ssagen.SymABIs, profile *pgoir.Profile) {
	c.PrepareFrontend(symABIs, profile)
	c.CompileDeclarations(symABIs, profile)
	c.FinishPackage()
}

// CheckPackage runs Main's final large-frame, function-stack and queue checks.
// The native driver invokes these after archive writing. Callers that buffer
// archive output must withhold that output if these checks report diagnostics.
func (c *Context) CheckPackage() {
	if c == nil || c.Backend == nil || !c.finishStarted || c.finishRunning || c.finishFailed || !c.Writer.Link.ObjectNumbered() {
		panic("final compiler checks require a completed package")
	}
	c.checkPackage()
}

// WriteAssemblyHeader emits the pinned assembler constants and type offsets.
// Call after the archive and before CheckPackage. The caller owns output.
// An attempted write consumes the stage, including when output fails.
func (c *Context) WriteAssemblyHeader(output io.WriteSeeker) (err error) {
	if c == nil || c.Package == nil || c.assemblyStarted || c.output != nil || output == nil || c.archiveModes != CombinedObject || c.archiveFailed || c.finishFailed || c.errors() != 0 || c.Package.Noder.Flag.AsmHdr == "" {
		return fmt.Errorf("assembly header requires a completed archive and enabled header output")
	}
	c.assemblyStarted, c.assemblyRunning = true, true
	c.output = &archiveOutput{WriteSeeker: output}
	defer func() {
		c.output = nil
		c.assemblyRunning = false
		if p := recover(); p != nil {
			if e, ok := p.(*bio.OutputError); ok {
				err = e
			} else {
				panic(p)
			}
		}
	}()
	c.dumpasmhdr()
	return nil
}

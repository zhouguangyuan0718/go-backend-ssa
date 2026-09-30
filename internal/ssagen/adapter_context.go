package ssagen

import (
	"fmt"
	"go-backend-ssa/diagnostic"
	"go-backend-ssa/internal/cmd/obj"
	"go-backend-ssa/internal/cmd/objabi"
	"go-backend-ssa/internal/cmd/src"
	"go-backend-ssa/internal/cmd/sys"
	"go-backend-ssa/internal/go/buildcfg"
	runtimegc "go-backend-ssa/internal/go/runtime/gc"
	errorspkg "go-backend-ssa/internal/go/types/errors"
	"go-backend-ssa/internal/inline"
	"go-backend-ssa/internal/ir"
	"go-backend-ssa/internal/liveness"
	"go-backend-ssa/internal/objw"
	"go-backend-ssa/internal/pgoir"
	"go-backend-ssa/internal/ssa"
	"go-backend-ssa/internal/typecheck"
	"go-backend-ssa/internal/types"
	"io"
	"io/fs"
	"sync"
)

// Context binds native ABI wrappers, target initialization and SSA frontend
// services to one compiler graph. Set SSA and Arch before InitConfig, after
// type and runtime declarations have been initialized. Phase operations within
// one graph are serialized; separate contexts can compile independently.
type Context struct {
	Inline                               *inline.Context
	constructionCapture                  *constructionState
	globalMapInitLsyms                   map[*obj.LSym]struct{}
	largeStackFramesMu                   sync.Mutex
	largeStackFrames                     []largeStack
	nowritebarrierrecCheck               *nowritebarrierrecChecker
	pkgSpecials                          *objabi.Context
	mallocGC                             *runtimegc.Context
	capVar                               *ir.Name
	deferBitsVar                         *ir.Name
	f32_u32                              f2uCvtTab
	f32_u64                              f2uCvtTab
	f64_u32                              f2uCvtTab
	f64_u64                              f2uCvtTab
	fpConvOpToSSA                        map[twoTypes]twoOpsAndType
	fpConvOpToSSA32                      map[twoTypes]twoOpsAndType
	hashVar                              *ir.Name
	lenVar                               *ir.Name
	memVar                               *ir.Name
	okVar                                *ir.Name
	opToSSA                              map[opAndType]ssa.Op
	ptrVar                               *ir.Name
	shiftOpToSSA                         map[opAndTwoTypes]ssa.Op
	typVar                               *ir.Name
	u32_f32                              u322fcvtTab
	u32_f64                              u322fcvtTab
	u64_f32                              u642fcvtTab
	u64_f64                              u642fcvtTab
	uint64fpConvOpToSSA                  map[twoTypes]twoOpsAndType
	intrinsics                           intrinsicBuilders
	ssaVecBySize                         map[int64]*types.Type
	cvtVToMaskOpcodes, cvtMaskToVOpcodes map[int]map[int]ssa.Op
	architectures                        *sys.Context
	GO386, GOMIPS, GOMIPS64              string
	GOARM64                              buildcfg.Goarm64Features
	MakeDumpDirectory                    func(string, fs.FileMode) error

	// DumpFunction and DumpDirectory supply native GOSSAFUNC and GOSSADIR inputs.
	DumpFunction, DumpDirectory string
	OpenSource                  func(string) (io.ReadCloser, error)
	ExpandSourcePath            func(string) string
	ssaDump, ssaDir, ssaDumpCFG string
	ssaDumpStdout               bool
	ssaDumpInlined              []*ir.Func
	Liveness                    *liveness.Context
	ptrSize, regSize            int
	maxWidth                    int64
	SSA                         *ssa.Context
	Arch                        ArchInfo
	ssaConfig                   *ssa.Config
	ssaCaches                   []ssa.Cache
	BoundsCheckFunc             [ssa.BoundsKindCount]*obj.LSym
	softFloatOps                map[ssa.Op]sfRtCallDef
	deferType                   *types.Type
	Typecheck                   *typecheck.Context
	// ReadFile supplies symabis input. There is no implicit filesystem fallback.
	ReadFile func(string) ([]byte, error)
}

func NewContext(tc *typecheck.Context) *Context {
	if tc == nil || tc.IR == nil {
		panic("ABI generation requires a compiler graph")
	}
	c := &Context{mallocGC: runtimegc.NewContext(), pkgSpecials: objabi.NewContext()}
	c.Typecheck = tc
	c.ptrSize, c.regSize, c.maxWidth = tc.IR.Types.PointerSize(), tc.IR.Types.RegisterSize(), tc.IR.Types.MaxWidth()
	if tc.IR.Base.Ctxt.Arch != nil {
		c.architectures = sys.ContextOfArch(tc.IR.Base.Ctxt.Arch.Arch)
	}
	c.initialize()
	return c
}
func (c *Context) readFile(name string) ([]byte, error) {
	if c.ReadFile == nil {
		return nil, fmt.Errorf("symabis input reader is not configured")
	}
	return c.ReadFile(name)
}
func (c *Context) fatalLogf(f string, a ...any) { panic(fmt.Errorf(f, a...)) }
func (c *Context) fatalf(f string, a ...any)    { panic(fmt.Errorf(f, a...)) }
func (c *Context) fatalfAt(pos src.XPos, f string, a ...any) {
	panic(fmt.Errorf("%s: %s", c.Typecheck.IR.Base.FmtPos(pos), fmt.Sprintf(f, a...)))
}
func (c *Context) traceWriter() io.Writer {
	if c.Typecheck.IR.TraceWriter != nil {
		return c.Typecheck.IR.TraceWriter
	}
	return io.Discard
}
func (c *Context) errorfAt(pos src.XPos, code errorspkg.Code, f string, a ...any) {
	g := c.Typecheck.IR
	g.Diagnostics.Add(diagnostic.Error, code.String(), g.Base.Ctxt.PosTable.Pos(pos), fmt.Sprintf(f, a...))
}
func (c *Context) checkSym(s *types.Sym) {
	if s == nil || s.Universe() != c.Typecheck.IR.Types {
		panic("ABI symbol belongs to another compilation")
	}
}
func (c *Context) checkWrapper(fn *ir.Func, _ obj.ABI) {
	if fn == nil || !c.Typecheck.IR.OwnsNode(fn) {
		panic("ABI function belongs to another compilation")
	}
}
func (c *Context) checkPackage() {
	if c.Typecheck.Target == nil {
		panic("ABI generation requires a target package")
	}
	for _, fn := range c.Typecheck.Target.Funcs {
		c.checkWrapper(fn, 0)
	}
}

// NewFrontend binds the native SSA frontend to an existing declared IR function.
func (c *Context) NewFrontend(fn *ir.Func, logging bool) ssa.Frontend {
	c.checkFrontendFunc(fn)
	return &ssafn{compilation: c, curfn: fn, log: logging}
}
func (c *Context) checkFrontendFunc(fn *ir.Func) {
	c.checkBackend()
	if fn == nil || !c.Typecheck.IR.OwnsNode(fn) || fn.Type() == nil || fn.Type().Universe() != c.Typecheck.IR.Types || fn.Dcl == nil {
		panic("SSA frontend requires a declared function from this compilation")
	}
}
func (c *Context) checkBackend() {
	if c == nil || c.Typecheck == nil || c.Typecheck.IR == nil || c.SSA == nil || c.SSA.IR != c.Typecheck.IR || c.SSA.Types != c.Typecheck.IR.Types || c.SSA.ReflectData == nil || c.SSA.ReflectData.Typecheck != c.Typecheck || c.SSA.ReflectData.StaticData == nil || c.SSA.ReflectData.StaticData.IR != c.Typecheck.IR {
		panic("ssagen requires matching SSA and frontend services")
	}
}
func (c *Context) checkInitConfig() {
	c.checkBackend()
	if c.Arch.LinkArch != c.Typecheck.IR.Base.Ctxt.Arch || c.Arch.LinkArch == nil || c.Typecheck.IR.Base.Flag.LowerC <= 0 || c.Typecheck.IR.Pkgs.Runtime == nil {
		panic("ssagen configuration requires matching target, workers and runtime symbols")
	}
}
func (c *Context) checkSplitSlot(parent *ssa.LocalSlot, suffix string, offset int64, typ *types.Type) {
	c.checkBackend()
	if parent == nil || parent.N == nil || !c.Typecheck.IR.OwnsNode(parent.N) || parent.Type == nil || parent.Type.Universe() != c.Typecheck.IR.Types || typ == nil || typ.Universe() != c.Typecheck.IR.Types {
		panic("SSA slot belongs to another compilation")
	}
}
func (c *Context) checkStringData(string) { c.checkBackend() }
func (c *Context) tracePrintf(format string, args ...any) (int, error) {
	return fmt.Fprintf(c.traceWriter(), format, args...)
}
func (c *Context) warnfAt(pos src.XPos, format string, args ...any) {
	g := c.Typecheck.IR
	g.Diagnostics.Add(diagnostic.Warning, "", g.Base.Ctxt.OutermostPos(pos), fmt.Sprintf(format, args...))
}

// SSAConfig returns the configuration prepared by native InitConfig.
func (c *Context) SSAConfig() *ssa.Config { return c.ssaConfig }

// AllocFrame runs native frame layout after SSA compilation and returns its
// total size, pointer-containing prefix size and required alignment.
func (c *Context) AllocFrame(f *ssa.Func) (size, pointerSize, alignment int64) {
	c.checkFrame(f)
	fe := f.Frontend().(*ssafn)
	fe.AllocFrame(f)
	return fe.stksize, fe.stkptrsize, fe.stkalign
}
func (c *Context) checkFrame(f *ssa.Func) {
	c.checkBackend()
	if c.Liveness == nil || c.Liveness.SSA != c.SSA || f == nil || f.Config != c.ssaConfig || f.RegAlloc == nil {
		panic("frame layout requires compiled SSA and matching liveness services")
	}
	fe, ok := f.Frontend().(*ssafn)
	if !ok || fe.compilation != c || fe.curfn == nil || !c.Typecheck.IR.OwnsNode(fe.curfn) {
		panic("frame frontend belongs to another compilation")
	}
	if debug, ok := fe.curfn.DebugInfo.(*ssa.FuncDebug); !ok || debug == nil {
		panic("frame layout requires native function debug metadata")
	}
	checkName := func(n *ir.Name) {
		if n == nil || !c.Typecheck.IR.OwnsNode(n) || n.Type() == nil || n.Type().Universe() != c.Typecheck.IR.Types {
			panic("frame variable belongs to another compilation")
		}
	}
	for _, n := range fe.curfn.Dcl {
		checkName(n)
	}
	for _, loc := range f.RegAlloc {
		if slot, ok := loc.(ssa.LocalSlot); ok {
			checkName(slot.N)
		}
	}
	for _, b := range f.Blocks {
		for _, v := range b.Values {
			if n, ok := v.Aux.(*ir.Name); ok {
				checkName(n)
			}
		}
	}
}

func (c *Context) dumpEnvironment(key string) string {
	switch key {
	case "GOSSAFUNC":
		return c.DumpFunction
	case "GOSSADIR":
		return c.DumpDirectory
	}
	panic("unsupported ssagen environment input")
}
func (c *Context) openSource(path string) (io.ReadCloser, error) {
	if c.OpenSource == nil {
		panic("SSA source display requires an input provider")
	}
	return c.OpenSource(path)
}
func (c *Context) expandSourcePath(path string) string {
	if c.ExpandSourcePath == nil {
		panic("SSA source display requires a path expansion provider")
	}
	return c.ExpandSourcePath(path)
}

func (c *Context) checkDumpInline(fn *ir.Func) {
	if fn == nil || !c.Typecheck.IR.OwnsNode(fn) {
		panic("SSA inline source belongs to another compilation")
	}
}

func (c *Context) assertCondition(ok bool) {
	if !ok {
		c.fatalf("assertion failed")
	}
}
func (c *Context) warn(format string, args ...any) {
	c.warnfAt(c.Typecheck.IR.Types.Pos, format, args...)
}
func (c *Context) makeDumpDirectory(path string, mode fs.FileMode) error {
	if c.MakeDumpDirectory == nil {
		panic("SSA HTML output requires a directory provider")
	}
	return c.MakeDumpDirectory(path, mode)
}
func (c *Context) checkIntrinsicTables() {
	c.checkBackend()
	if c.architectures == nil || c.Arch.LinkArch != c.Typecheck.IR.Base.Ctxt.Arch || sys.ContextOfArch(c.Arch.LinkArch.Arch) != c.architectures {
		panic("intrinsic tables require matching canonical target descriptors")
	}
	c.Typecheck.IR.IsIntrinsicCall = c.IsIntrinsicCall
	c.Typecheck.IR.IsIntrinsicSym = c.IsIntrinsicSym
}
func (c *Context) checkBuildSSA(fn *ir.Func, worker int, isPgoHot bool) {
	c.checkFrontendFunc(fn)
	if c.ssaConfig == nil || worker < 0 || worker >= len(c.ssaCaches) || c.intrinsics == nil || c.Liveness == nil || c.Liveness.SSA != c.SSA {
		panic("IR-to-SSA requires initialized target, intrinsics, worker and liveness services")
	}
	// Validate graph ownership before native construction changes symbol, debug,
	// declaration, SSA-cache or frame state. Hidden IR links may form cycles.
	seen := map[ir.Node]bool{}
	var visit func(ir.Node) bool
	visit = func(n ir.Node) bool {
		if n == nil || seen[n] {
			return false
		}
		seen[n] = true
		if !c.Typecheck.IR.OwnsNode(n) || n.Type() != nil && n.Type().Universe() != c.Typecheck.IR.Types {
			panic("IR-to-SSA node belongs to another compilation")
		}
		ir.DoChildrenWithHidden(n, visit)
		return false
	}
	visit(fn)
	for _, n := range fn.Dcl {
		visit(n)
	}

}

// BuildSSA runs native IR construction, phi placement, SSA compilation, frame
// allocation, defer metadata and incoming register spill recording for fn.
// Run InitConfig and InitTables after configuring target services. The function
// must have completed frontend lowering. Serialize calls within one Context.
// Consume the returned graph before reusing the same worker cache.
func (c *Context) BuildSSA(fn *ir.Func, worker int, isPgoHot bool) *ssa.Func {
	return c.buildssa(fn, worker, isPgoHot)
}

func (c *Context) checkIntrinsicBody(fn *ir.Func) { c.checkFrontendFunc(fn) }
func (c *Context) checkIntrinsicSymbol(sym *types.Sym) {
	if sym != nil && sym.Pkg != nil && sym.Universe() != c.Typecheck.IR.Types {
		panic("intrinsic symbol belongs to another compilation")
	}
}
func (c *Context) checkIntrinsicCall(n *ir.CallExpr) {
	if n != nil && !c.Typecheck.IR.OwnsNode(n) {
		panic("intrinsic call belongs to another compilation")
	}
}

func (c *Context) tracePrint(args ...any) (int, error) { return fmt.Fprint(c.traceWriter(), args...) }
func (c *Context) tracePrintln(args ...any) (int, error) {
	return fmt.Fprintln(c.traceWriter(), args...)
}

func (c *Context) checkGenSSA(f *ssa.Func, pp *objw.Progs) {
	c.checkBackend()
	if f == nil || f.Config != c.ssaConfig || f.RegAlloc == nil || f.Frontend() == nil || pp == nil || pp.CurFunc != f.Frontend().Func() || c.Liveness == nil || c.Liveness.SSA != c.SSA {
		panic("instruction emission requires a compiled function from this backend")
	}
	fe, ok := f.Frontend().(*ssafn)
	if !ok || fe.compilation != c || fe.stkalign == 0 || !c.SSA.ReflectData.Runtime.Writer.OwnsProgs(pp) {
		panic("instruction emission function or buffer belongs to another compilation")
	}
	if c.Arch.LinkArch != c.Typecheck.IR.Base.Ctxt.Arch || c.Arch.SSAMarkMoves == nil || c.Arch.SSAGenValue == nil || c.Arch.SSAGenBlock == nil || c.Arch.Ginsnop == nil || c.Arch.ZeroRange == nil || c.Arch.SpillArgReg == nil || c.Arch.LoadRegResult == nil {
		panic("instruction emission requires initialized architecture handlers")
	}
}

// GenSSA emits native instructions and GC, argument, frame and variable-location
// metadata into pp. Consume or assemble the instructions before freeing pp or
// reusing its worker cache. This operation does not assemble machine-code bytes.
func (c *Context) GenSSA(f *ssa.Func, pp *objw.Progs) { c.genssa(f, pp) }

func (c *Context) checkCompile(fn *ir.Func, worker int, profile *pgoir.Profile) {
	c.checkBuildSSA(fn, worker, false)
	if c.Inline == nil || c.Inline.Typecheck != c.Typecheck || (profile != nil && !c.Inline.PGO.OwnsProfile(profile)) {
		panic("function compilation requires matching inline services")
	}
}

// OwnsSymABIs reports whether ABI input belongs to this backend.
func (c *Context) OwnsSymABIs(s *SymABIs) bool { return s != nil && s.compilation == c }

// WorkerCount returns the cache count fixed by InitConfig.
func (c *Context) WorkerCount() int { return len(c.ssaCaches) }

// Construction owns a graph at the native boundary after phi insertion and
// before SSA compilation. Its cache is not reused by later function builds.
// Operations sharing a compiler Context must be serialized. Keep the compiler
// configuration and frontend IR unchanged until ContinueNative completes.
// This low-level function boundary is not the standalone package Build API.
type Construction struct{ state *constructionState }

// Handles may be copied by external callers. Keep consumption and completion
// on shared private state so every copy observes the same one-shot operation.
type constructionState struct {
	owner        *Context
	function     *ssa.Func
	continuation func() *ssa.Func
	started      bool
	complete     bool
}

func (c *Construction) Func() *ssa.Func {
	if c == nil || c.state == nil || c.state.function == nil {
		panic("uninitialized SSA construction")
	}
	return c.state.function
}

// ContinueNative consumes the continuation exactly once, including on failure.
// It runs the original SSA pipeline, frame allocation, defer data and incoming
// register-spill recording, then returns the same graph. GenSSA may consume it.
func (h *Construction) ContinueNative() *ssa.Func {
	if h == nil || h.state == nil {
		panic("uninitialized SSA construction")
	}
	c := h.state
	if c.owner == nil || c.function == nil || c.continuation == nil || c.started || c.owner.constructionCapture != nil || c.function.Config != c.owner.ssaConfig || c.owner.Typecheck.IR.Base.Ctxt.ObjectNumbered() {
		panic("SSA continuation is unavailable or its compiler state changed")
	}
	c.started = true
	continuation := c.continuation
	c.continuation = nil
	f := continuation()
	if f != c.function {
		panic("SSA continuation changed the function identity")
	}
	c.complete = true
	return f
}

// NativeCompleted reports successful completion of the original native tail.
func (c *Construction) NativeCompleted() bool { return c != nil && c.state != nil && c.state.complete }

// ConstructSSA runs the original construction prefix with an exclusively owned
// cache and stops after phi insertion. The returned graph survives later calls
// to both ConstructSSA and the worker-cache-based BuildSSA on this Context.
// fn must already have completed normal frontend lowering and symbol setup.
func (c *Context) ConstructSSA(fn *ir.Func, isPgoHot bool) *Construction {
	c.checkBuildSSA(fn, 0, isPgoHot)
	if c.constructionCapture != nil {
		panic("SSA construction is already active")
	}
	previous := c.ssaCaches
	// Construction is serial and uses worker zero. Cache contains large inline
	// value/block arrays; retain one cache per graph, not one per native worker.
	c.ssaCaches = make([]ssa.Cache, 1)
	capture := &constructionState{owner: c}
	c.constructionCapture = capture
	defer func() { c.ssaCaches = previous; c.constructionCapture = nil }()
	f := c.buildssa(fn, 0, isPgoHot)
	if capture.function != f || capture.continuation == nil {
		panic("native SSA construction boundary was not reached")
	}
	return &Construction{state: capture}
}

// The generator preserves the entire original suffix in continuation. Native
// callers run it synchronously, in the same scope as the original function.
func (c *Context) finishSSAConstruction(s *state, continuation func() *ssa.Func) *ssa.Func {
	capture := c.constructionCapture
	if capture == nil {
		return continuation()
	}
	if s == nil || s.compilation != c || s.f == nil || capture.owner != c || capture.function != nil {
		panic("invalid SSA construction capture")
	}
	capture.function = s.f
	// buildssa's original deferred popLine executes when construction returns.
	// Restore exactly that saved scope during the delayed native suffix, without
	// rerunning construction or introducing another source-position update.
	lines := append([]src.XPos(nil), s.line...)
	capture.continuation = func() *ssa.Func {
		previous := s.line
		s.line = lines
		defer func() { s.line = previous }()
		return continuation()
	}
	return s.f
}

package ssagen

import (
	"fmt"
	"go-backend-ssa/diagnostic"
	"go-backend-ssa/internal/cmd/obj"
	"go-backend-ssa/internal/cmd/src"
	errorspkg "go-backend-ssa/internal/go/types/errors"
	"go-backend-ssa/internal/ir"
	"go-backend-ssa/internal/liveness"
	"go-backend-ssa/internal/ssa"
	"go-backend-ssa/internal/typecheck"
	"go-backend-ssa/internal/types"
	"io"
)

// Context binds native ABI wrappers, target initialization and SSA frontend
// services to one compiler graph. Set SSA and Arch before InitConfig, after
// type and runtime declarations have been initialized. Phase operations within
// one graph are serialized; separate contexts can compile independently.
type Context struct {
	Liveness         *liveness.Context
	ptrSize, regSize int
	maxWidth         int64
	SSA              *ssa.Context
	Arch             ArchInfo
	ssaConfig        *ssa.Config
	ssaCaches        []ssa.Cache
	BoundsCheckFunc  [ssa.BoundsKindCount]*obj.LSym
	softFloatOps     map[ssa.Op]sfRtCallDef
	deferType        *types.Type
	Typecheck        *typecheck.Context
	// ReadFile supplies symabis input. There is no implicit filesystem fallback.
	ReadFile func(string) ([]byte, error)
}

func NewContext(tc *typecheck.Context) *Context {
	if tc == nil || tc.IR == nil {
		panic("ABI generation requires a compiler graph")
	}
	c := newContext()
	c.Typecheck = tc
	c.ptrSize, c.regSize, c.maxWidth = tc.IR.Types.PointerSize(), tc.IR.Types.RegisterSize(), tc.IR.Types.MaxWidth()
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

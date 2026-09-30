package walk

import (
	"fmt"
	"go-backend-ssa/diagnostic"
	"go-backend-ssa/internal/cmd/src"
	"go-backend-ssa/internal/compare"
	"go-backend-ssa/internal/escape"
	"go-backend-ssa/internal/ir"
	"go-backend-ssa/internal/noder"
	"go-backend-ssa/internal/typecheck"
	"go-backend-ssa/internal/types"
	"io"
)

// Context owns native walk caches and counters for a single compilation graph.
// Walk follows escape and slice, mutates the input IR, and leaves CurFunc set,
// matching the native compiler's contract with its driver.
type Context struct {
	Typecheck                                    *typecheck.Context
	Noder                                        *noder.Context
	Escape                                       *escape.Context
	Compare                                      *compare.Context
	SoftFloat                                    bool
	ptrSize, regSize                             int
	math_MulUintptr                              *types.Sym
	hooks                                        map[string]hookInfo
	typeAssertGen, interfaceSwitchGen            int
	scase                                        *types.Type
	rtypeHashField, itabHashField, itabTypeField *types.Field
	mapaccess1, mapaccess2, mapassign, mapdelete mapnames
	staticValues                                 map[ir.Node]ir.Node
	shapeConvSources                             map[*ir.Name]*types.Type
}

func NewContext(nd *noder.Context, esc *escape.Context) *Context {
	if nd == nil || nd.Typecheck == nil || nd.Typecheck.IR == nil || nd.Types != nd.Typecheck.IR.Types || esc == nil || esc.Typecheck != nd.Typecheck || nd.ReflectData == nil || nd.ReflectData.Typecheck != nd.Typecheck || nd.StaticInit == nil || nd.StaticInit.Typecheck != nd.Typecheck || nd.StaticInit.ReflectData != nd.ReflectData || nd.Typecheck.IR.Base.Ctxt.Arch == nil {
		panic("walk requires shared reader, escape, typecheck and static data contexts")
	}
	c := &Context{Noder: nd, Escape: esc, Typecheck: nd.Typecheck, Compare: compare.NewContext(nd.Typecheck), ptrSize: nd.Types.PointerSize(), regSize: nd.Types.RegisterSize()}
	c.initialize()
	return c
}
func (c *Context) checkFunc(fn *ir.Func) {
	if fn == nil || !c.Typecheck.IR.OwnsNode(fn) {
		panic("walk function belongs to another compilation")
	}
}
func (c *Context) fatalf(f string, a ...any) { panic(fmt.Errorf(f, a...)) }
func (c *Context) fatalfAt(pos src.XPos, f string, a ...any) {
	panic(fmt.Errorf("%s: %s", c.Typecheck.IR.Base.FmtPos(pos), fmt.Sprintf(f, a...)))
}
func (c *Context) assertCondition(ok bool) {
	if !ok {
		c.fatalf("assertion failed")
	}
}
func (c *Context) assertfAt(ok bool, pos src.XPos, f string, a ...any) {
	if !ok {
		c.fatalfAt(pos, f, a...)
	}
}
func (c *Context) errorf(f string, a ...any) {
	g := c.Typecheck.IR
	g.Diagnostics.Add(diagnostic.Error, "", g.Base.Ctxt.OutermostPos(g.Types.Pos), fmt.Sprintf(f, a...))
}
func (c *Context) warn(f string, a ...any) { c.warnfAt(c.Typecheck.IR.Types.Pos, f, a...) }
func (c *Context) warnfAt(pos src.XPos, f string, a ...any) {
	g := c.Typecheck.IR
	g.Diagnostics.Add(diagnostic.Warning, "", g.Base.Ctxt.OutermostPos(pos), fmt.Sprintf(f, a...))
}
func (c *Context) errors() int {
	n := 0
	g := c.Typecheck.IR
	for _, ds := range [][]diagnostic.Diagnostic{g.Diagnostics.Diagnostics(), g.Types.Diagnostics.Diagnostics()} {
		for _, d := range ds {
			if d.Severity == diagnostic.Error {
				n++
			}
		}
	}
	return n
}
func (c *Context) traceWriter() io.Writer {
	if w := c.Typecheck.IR.TraceWriter; w != nil {
		return w
	}
	return io.Discard
}
func (c *Context) tracePrintf(f string, a ...any) (int, error) {
	return fmt.Fprintf(c.traceWriter(), f, a...)
}

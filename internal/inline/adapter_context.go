package inline

import (
	"fmt"
	"go-backend-ssa/internal/cmd/src"
	"go-backend-ssa/internal/inline/inlheur"
	"go-backend-ssa/internal/ir"
	"go-backend-ssa/internal/logopt"
	"go-backend-ssa/internal/pgoir"
	"go-backend-ssa/internal/typecheck"
	"io"
)

// Context owns the native inlining policy and hot-call maps. Its dependencies
// share one compiler graph; separate graphs can be analyzed concurrently.
type Context struct {
	Typecheck                            *typecheck.Context
	PGO                                  *pgoir.Context
	Heuristics                           *inlheur.Context
	Log                                  *logopt.Context
	candHotCalleeMap                     map[*pgoir.IRNode]struct{}
	hasHotCall                           map[*ir.Func]struct{}
	candHotEdgeMap                       map[pgoir.CallSiteInfo]struct{}
	inlineHotCallSiteThresholdPercent    float64
	inlineCDFHotCallSiteThresholdPercent float64
	inlineHotMaxBudget                   int32
}

func NewContext(tc *typecheck.Context, heur *inlheur.Context, log *logopt.Context) *Context {
	if tc == nil || heur == nil || heur.Typecheck != tc || log == nil {
		panic("inlining requires shared compiler and heuristic contexts and an event collector")
	}
	c := newContext()
	c.Typecheck = tc
	c.PGO = heur.PGO
	c.Heuristics = heur
	c.Log = log
	return c
}
func (c *Context) checkFunc(fn *ir.Func) {
	if fn == nil || !c.Typecheck.IR.OwnsNode(fn) {
		panic("inline function belongs to another compilation")
	}
}
func (c *Context) checkProfile(p *pgoir.Profile) {
	if p != nil && !c.PGO.OwnsProfile(p) {
		panic("inline profile belongs to another compilation")
	}
}
func (c *Context) checkFuncProfile(fn *ir.Func, p *pgoir.Profile) { c.checkFunc(fn); c.checkProfile(p) }
func (c *Context) checkFuncsProfile(fns []*ir.Func, p *pgoir.Profile) {
	for _, fn := range fns {
		c.checkFunc(fn)
	}
	c.checkProfile(p)
}
func (c *Context) checkCallProfile(fn *ir.Func, call *ir.CallExpr, p *pgoir.Profile) {
	c.checkFuncProfile(fn, p)
	if call == nil || !c.Typecheck.IR.OwnsNode(call) {
		panic("inline call belongs to another compilation")
	}
}
func (c *Context) fatalf(f string, a ...any) { panic(fmt.Errorf(f, a...)) }
func (c *Context) fatalfAt(pos src.XPos, f string, a ...any) {
	panic(fmt.Errorf("%s: %s", c.Typecheck.IR.Base.FmtPos(pos), fmt.Sprintf(f, a...)))
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
func (c *Context) tracePrintln(a ...any) (int, error) { return fmt.Fprintln(c.traceWriter(), a...) }

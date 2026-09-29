package inlheur

import (
	"fmt"
	"go-backend-ssa/internal/cmd/src"
	"go-backend-ssa/internal/ir"
	"go-backend-ssa/internal/pgoir"
	"go-backend-ssa/internal/typecheck"
	"io"
)

// Context owns function properties, scoring adjustments and call-site caches.
// It shares the caller's compiler graph; independent graphs may run concurrently.
type Context struct {
	mayMustAdj              [4]struct{ may, must scoreAdjustTyp }
	Typecheck               *typecheck.Context
	PGO                     *pgoir.Context
	fpmap                   map[*ir.Func]fnInlHeur
	adjValues               map[scoreAdjustTyp]int
	resultFlagToPositiveAdj map[ResultPropBits]scoreAdjustTyp
	paramFlagToPositiveAdj  map[ParamPropBits]scoreAdjustTyp
	callSiteTab             CallSiteTab
	scoreCallsCache         scoreCallsCacheType
	allCallSites            CallSiteTab
}

func NewContext(tc *typecheck.Context, pg *pgoir.Context) *Context {
	if tc == nil || pg == nil || pg.Typecheck != tc {
		panic("inline heuristics require shared compiler and PGO contexts")
	}
	c := newContext()
	c.Typecheck = tc
	c.PGO = pg
	return c
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

func (c *Context) checkFunc(fn *ir.Func) {
	if fn == nil || fn.Sym() == nil || fn.Sym().Universe() != c.Typecheck.IR.Types {
		panic("inline function belongs to another compilation")
	}
}
func (c *Context) checkAnalyze(fn *ir.Func, canInline func(*ir.Func), budgetForFunc func(*ir.Func) int32, inlineMaxBudget int) {
	c.checkFunc(fn)
}
func (c *Context) checkCallScore(fn *ir.Func, call *ir.CallExpr) {
	c.checkFunc(fn)
	if call != nil && call.Type() != nil && call.Type().Universe() != c.Typecheck.IR.Types {
		panic("inline call belongs to another compilation")
	}
}
func (c *Context) checkProps(fn *ir.Func, props *FuncProps) { c.checkFunc(fn) }

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
	dumpBuffer              map[*ir.Func]fnInlHeur
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

func (c *Context) checkDumpFunc(fn *ir.Func, dumpfile string) {
	if fn != nil {
		c.checkFunc(fn)
	}
}

// emitDump replaces only the command's file-opening boundary. The native
// ordering and formatting below are retained; the host supplies TraceWriter.
// dumpfile remains a driver enable flag and is never opened by the library.
func (c *Context) emitDump(dumpfile string) {
	outf := c.traceWriter()
	dumpFilePreamble(outf)
	atline := map[uint]uint{}
	sl := make([]fnInlHeur, 0, len(c.dumpBuffer))
	for _, e := range c.dumpBuffer {
		sl = append(sl, e)
		atline[e.line]++
	}
	sl = sortFnInlHeurSlice(sl)
	prevline := uint(0)
	for _, entry := range sl {
		idx := uint(0)
		if prevline == entry.line {
			idx++
		}
		prevline = entry.line
		if err := c.dumpFnPreamble(outf, &entry, nil, idx, atline[entry.line]); err != nil {
			c.fatalf("function props dump: %v\n", err)
		}
	}
	c.dumpBuffer = nil
}

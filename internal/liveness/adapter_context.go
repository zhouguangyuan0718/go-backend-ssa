package liveness

import (
	"fmt"
	"go-backend-ssa/diagnostic"
	"go-backend-ssa/internal/abi"
	"go-backend-ssa/internal/cmd/src"
	errorspkg "go-backend-ssa/internal/go/types/errors"
	"go-backend-ssa/internal/ir"
	"go-backend-ssa/internal/objw"
	"go-backend-ssa/internal/ssa"
	"io"
)

// Context binds native liveness and local merging to one SSA compilation.
// Operations sharing a graph are serialized. Independent contexts are isolated.
type Context struct {
	SSA *ssa.Context
	// ClobberDeadHash supplies the native GOCLOBBERDEADHASH input explicitly.
	ClobberDeadHash string
	ptrSize         int
}

func NewContext(s *ssa.Context) *Context {
	if s == nil || s.IR == nil || s.Types == nil || s.IR.Types != s.Types {
		panic("liveness requires a matching SSA compilation")
	}
	c := newContext()
	c.SSA, c.ptrSize = s, s.Types.PointerSize()
	return c
}
func (c *Context) fatalf(format string, args ...any) { panic(fmt.Errorf(format, args...)) }
func (c *Context) fatalfAt(pos src.XPos, format string, args ...any) {
	panic(fmt.Errorf("%s: %s", c.SSA.IR.Base.FmtPos(pos), fmt.Sprintf(format, args...)))
}
func (c *Context) traceWriter() io.Writer {
	if c.SSA.IR.TraceWriter != nil {
		return c.SSA.IR.TraceWriter
	}
	return io.Discard
}
func (c *Context) tracePrintf(format string, args ...any) (int, error) {
	return fmt.Fprintf(c.traceWriter(), format, args...)
}
func (c *Context) clobberDeadHash(name string) string {
	if name != "GOCLOBBERDEADHASH" {
		panic("unreviewed liveness environment key")
	}
	return c.ClobberDeadHash
}
func (c *Context) checkMerge(fn *ir.Func, f *ssa.Func) {
	if c == nil || c.SSA == nil || fn == nil || !c.SSA.IR.OwnsNode(fn) || f == nil || f.Config == nil || f.Config.Ctxt() != c.SSA.IR.Base.Ctxt || f.Frontend() == nil || f.Frontend().Func() != fn {
		panic("local merging requires a function from this compilation")
	}
	for _, n := range fn.Dcl {
		if n == nil || !c.SSA.IR.OwnsNode(n) || n.Type() == nil || n.Type().Universe() != c.SSA.Types {
			panic("local merging declaration belongs to another compilation")
		}
	}
}

// Keep the native slice allocation, length and capacity. Bind every allocated
// element so later reslicing also retains the compilation's diagnostic writer.
func (c *Context) bindIntervalBuilders(values []IntervalsBuilder) []IntervalsBuilder {
	for i := range values[:cap(values)] {
		values[:cap(values)][i].compilation = c
	}
	return values
}

func (c *Context) tracePrint(args ...any) (int, error) { return fmt.Fprint(c.traceWriter(), args...) }
func (c *Context) tracePrintln(args ...any) (int, error) {
	return fmt.Fprintln(c.traceWriter(), args...)
}
func (c *Context) assertCondition(ok bool) {
	if !ok {
		c.fatalf("assertion failed")
	}
}
func (c *Context) warnfAt(p src.XPos, f string, a ...any) {
	c.SSA.IR.Diagnostics.Add(diagnostic.Warning, "", c.SSA.IR.Base.Ctxt.OutermostPos(p), fmt.Sprintf(f, a...))
}

func (c *Context) errorfAt(p src.XPos, code errorspkg.Code, f string, a ...any) {
	c.SSA.IR.Diagnostics.Add(diagnostic.Error, code.String(), c.SSA.IR.Base.Ctxt.PosTable.Pos(p), fmt.Sprintf(f, a...))
}

func (c *Context) checkArgLiveness(fn *ir.Func, f *ssa.Func, pp *objw.Progs) {
	c.checkMerge(fn, f)
	if c.SSA.ReflectData == nil || !c.SSA.ReflectData.Runtime.Writer.OwnsProgs(pp) || pp.CurFunc != fn {
		panic("liveness requires an owned instruction buffer")
	}
}
func (c *Context) checkCompute(fn *ir.Func, f *ssa.Func, stkptrsize int64, pp *objw.Progs, retLiveness bool) {
	c.checkArgLiveness(fn, f, pp)
}
func (c *Context) checkFuncMap(fn *ir.Func, info *abi.ABIParamResultInfo) {
	if c == nil || c.SSA == nil || fn == nil || !c.SSA.IR.OwnsNode(fn) || info == nil || info.Config().Universe() != c.SSA.Types {
		panic("function stack map belongs to another compilation")
	}
	for _, list := range [][]abi.ABIParamAssignment{info.InParams(), info.OutParams()} {
		for _, a := range list {
			if a.Type == nil || a.Type.Universe() != c.SSA.Types || a.Name != nil && !c.SSA.IR.OwnsNode(a.Name) {
				panic("function stack-map argument belongs to another compilation")
			}
		}
	}
}

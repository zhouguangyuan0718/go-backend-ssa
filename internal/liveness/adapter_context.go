package liveness

import (
	"fmt"
	"go-backend-ssa/internal/cmd/src"
	"go-backend-ssa/internal/ir"
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

package escape

import (
	"fmt"
	"go-backend-ssa/diagnostic"
	"go-backend-ssa/internal/cmd/src"
	errorspkg "go-backend-ssa/internal/go/types/errors"
	"go-backend-ssa/internal/ir"
	"go-backend-ssa/internal/logopt"
	"go-backend-ssa/internal/typecheck"
	"io"
)

// Context binds native escape analysis and its summary cache to one IR graph.
// Funcs runs once after deadlocals and before slice/walk. It mutates the native
// escape classifications, parameter notes, and closure capture metadata.
type Context struct {
	Typecheck    *typecheck.Context
	Log          *logopt.Context
	ptrSize      int
	leakTagCache map[leaks]string
}

func NewContext(tc *typecheck.Context, log *logopt.Context) *Context {
	if tc == nil || tc.IR == nil || log == nil {
		panic("escape analysis requires a compiler graph and optimizer log")
	}
	c := newContext()
	c.Typecheck = tc
	c.Log = log
	c.ptrSize = tc.IR.Types.PointerSize()
	tc.IR.EscFmt = Fmt
	return c
}
func (c *Context) checkNode(n ir.Node) {
	if n == nil || !c.Typecheck.IR.OwnsNode(n) {
		panic("escape node belongs to another compilation")
	}
}
func (c *Context) checkFuncs(fns []*ir.Func) {
	for _, fn := range fns {
		c.checkNode(fn)
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
func (c *Context) warnfAt(pos src.XPos, f string, a ...any) {
	g := c.Typecheck.IR
	g.Diagnostics.Add(diagnostic.Warning, "", g.Base.Ctxt.OutermostPos(pos), fmt.Sprintf(f, a...))
	g.Base.ReportWarning(pos, f, a...)
}
func (c *Context) errorfAt(pos src.XPos, code errorspkg.Code, f string, a ...any) {
	g := c.Typecheck.IR
	g.Diagnostics.Add(diagnostic.Error, code.String(), g.Base.Ctxt.OutermostPos(pos), fmt.Sprintf(f, a...))
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

func (c *Context) checkOptionalNode(n ir.Node) {
	if n != nil {
		c.checkNode(n)
	}
}

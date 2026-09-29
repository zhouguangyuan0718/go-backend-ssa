package devirtualize

import (
	"fmt"
	"go-backend-ssa/diagnostic"
	"go-backend-ssa/internal/cmd/src"
	"go-backend-ssa/internal/inline"
	"go-backend-ssa/internal/ir"
	"go-backend-ssa/internal/pgoir"
	"go-backend-ssa/internal/typecheck"
	"go-backend-ssa/internal/types"
	"io"
)

// Context owns the sentinel and dependencies of the native devirtualizer.
// Each State binds to this context on its first StaticCall, including a call
// that cannot be devirtualized. States and their graphs must not be shared
// between compilation instances.
type Context struct {
	Typecheck *typecheck.Context
	Inline    *inline.Context
	noType    types.Type
}

func NewContext(inl *inline.Context) *Context {
	if inl == nil || inl.Typecheck == nil {
		panic("devirtualization requires an inlining context")
	}
	c := newContext()
	c.Typecheck = inl.Typecheck
	c.Inline = inl
	return c
}

// StaticCall retains the native zero-value State workflow while checking graph
// ownership before binding the state or executing the native transformation.
func (c *Context) StaticCall(s *State, call *ir.CallExpr) {
	if c == nil || s == nil {
		panic("devirtualization requires context and state")
	}
	c.checkNode(call)
	if s.compilation == nil {
		s.compilation = c
	} else if s.compilation != c {
		panic("devirtualization state belongs to another compilation")
	}
	c.staticCall(s, call)
}
func (c *Context) checkNode(n ir.Node) {
	if n == nil || !c.Typecheck.IR.OwnsNode(n) {
		panic("devirtualization node belongs to another compilation")
	}
}
func (c *Context) checkInlinedCall(fn *ir.Func, call *ir.CallExpr, inlined *ir.InlinedCallExpr) {
	c.checkNode(fn)
	c.checkNode(call)
	c.checkNode(inlined)
}
func (c *Context) checkProfileGuided(fn *ir.Func, p *pgoir.Profile) {
	c.checkNode(fn)
	if p == nil || !c.Inline.PGO.OwnsProfile(p) {
		panic("devirtualization requires a profile from this compilation")
	}
}
func (c *Context) fatalf(f string, a ...any) { panic(fmt.Errorf(f, a...)) }
func (c *Context) fatalfAt(pos src.XPos, f string, a ...any) {
	panic(fmt.Errorf("%s: %s", c.Typecheck.IR.Base.FmtPos(pos), fmt.Sprintf(f, a...)))
}
func (c *Context) warnfAt(pos src.XPos, f string, a ...any) {
	g := c.Typecheck.IR
	g.Diagnostics.Add(diagnostic.Warning, "", g.Base.Ctxt.PosTable.Pos(pos), fmt.Sprintf(f, a...))
}
func (c *Context) warn(f string, a ...any) { c.warnfAt(c.Typecheck.IR.Types.Pos, f, a...) }
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

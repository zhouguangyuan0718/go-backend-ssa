package compare

import (
	"fmt"
	"go-backend-ssa/internal/cmd/obj"
	"go-backend-ssa/internal/ir"
	"go-backend-ssa/internal/typecheck"
	"go-backend-ssa/internal/types"
)

// Context binds native comparison construction to an existing compiler graph.
// Type-only queries retain their native signature and derive these inputs from t.
type Context struct {
	Typecheck *typecheck.Context
	Types     *types.Universe
	Ctxt      *obj.Link
	regSize   int
}

func NewContext(tc *typecheck.Context) *Context {
	if tc == nil || tc.IR == nil || tc.IR.Base.Ctxt.Arch == nil {
		panic("comparison construction requires a compiler graph with target architecture")
	}
	c := newContext()
	c.Typecheck = tc
	c.Types = tc.IR.Types
	c.Ctxt = tc.IR.Base.Ctxt
	c.regSize = c.Types.RegisterSize()
	return c
}
func typeContextOf(t *types.Type) *Context {
	if t == nil || t.Universe() == nil {
		panic("comparison requires an initialized compiler type")
	}
	u := t.Universe()
	c := &Context{Types: u, regSize: u.RegisterSize()}
	if link := u.LinkContext(); link != nil {
		c.Ctxt = link.Ctxt
	}
	return c
}
func (c *Context) checkCost(t *types.Type) {
	if t == nil || t.Universe() != c.Types || !t.IsStruct() {
		panic("comparison cost requires a struct from this compilation")
	}
	if c.Ctxt == nil || c.Ctxt.Arch == nil {
		panic("comparison cost requires target architecture")
	}
}
func (c *Context) checkMemrun(t *types.Type, start int) {
	c.checkCost(t)
	if start < 0 || start >= t.NumFields() {
		panic("comparison memory run starts outside struct")
	}
}
func (c *Context) checkNodes(a, b ir.Node) {
	if c.Typecheck == nil || a == nil || b == nil || !c.Typecheck.IR.OwnsNode(a) || !c.Typecheck.IR.OwnsNode(b) {
		panic("comparison node belongs to another compilation")
	}
}
func (c *Context) checkStruct(t *types.Type, a, b ir.Node) { c.checkCost(t); c.checkNodes(a, b) }
func (c *Context) fatalf(f string, a ...any)               { panic(fmt.Errorf(f, a...)) }

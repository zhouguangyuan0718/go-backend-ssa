package abi

import (
	"fmt"
	"go-backend-ssa/internal/ir"
	"go-backend-ssa/internal/types"
	"sync"
)

// Context owns the native synthetic ABI types for one compiler graph. Layout
// analysis within a graph is serial; separate graphs may run concurrently.
type Context struct {
	IR                                  *ir.Context
	regSize                             int
	synthOnce                           sync.Once
	synthSlice, synthString, synthIface *types.Type
}

func NewContext(graph *ir.Context) *Context {
	if graph == nil || graph.Types == nil {
		panic("ABI analysis requires an IR graph")
	}
	c := newContext()
	c.IR = graph
	c.regSize = graph.Types.RegisterSize()
	return c
}
func (c *Context) fatalf(f string, a ...any) { panic(fmt.Errorf(f, a...)) }
func (c *Context) checkType(t *types.Type) {
	if t == nil || t.Universe() != c.IR.Types {
		panic("ABI type belongs to another compilation")
	}
}
func (c *Context) checkTypes(params, results []*types.Type) {
	for _, ts := range [][]*types.Type{params, results} {
		for _, t := range ts {
			c.checkType(t)
		}
	}
}
func (c *Context) checkSignature(t *types.Type) {
	c.checkType(t)
	if t.Kind() != types.TFUNC {
		panic("ABI analysis requires a function signature")
	}
	for _, f := range t.RecvParamsResults() {
		c.checkType(f.Type)
		if f.Nname != nil {
			n, ok := f.Nname.(*ir.Name)
			if !ok || !c.IR.OwnsNode(n) {
				panic("ABI parameter name belongs to another compilation")
			}
		}
	}
}
func (c *Context) checkAnalyze(t *types.Type, setNname bool) { c.checkSignature(t) }
func (c *Context) checkAssignments(as []ABIParamAssignment) {
	for _, a := range as {
		if a.compilation != c {
			panic("ABI assignment belongs to another compilation")
		}
		assignmentContext(&a)
	}
}
func (c *Context) checkInfo(i *ABIParamResultInfo) {
	if i == nil || i.config == nil || i.config.compilation != c {
		panic("ABI result belongs to another compilation")
	}
}

func assignmentContext(a *ABIParamAssignment) *Context {
	if a == nil || a.compilation == nil {
		panic("ABI assignment has no compilation owner")
	}
	c := a.compilation
	c.checkType(a.Type)
	if a.Name != nil && !c.IR.OwnsNode(a.Name) {
		panic("ABI parameter name belongs to another compilation")
	}
	return c
}

// Universe identifies the compilation that owns the ABI configuration, including
// configurations whose signatures have no inputs or results.
func (c *ABIConfig) Universe() *types.Universe {
	if c == nil || c.compilation == nil {
		return nil
	}
	return c.compilation.IR.Types
}

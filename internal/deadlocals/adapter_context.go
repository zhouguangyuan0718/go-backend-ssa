package deadlocals

import "go-backend-ssa/internal/ir"

// Context binds the native unused-assignment pass to an existing IR graph.
// Each invocation keeps the original function-local visitor state.
type Context struct {
	IR *ir.Context
}

func NewContext(graph *ir.Context) *Context {
	if graph == nil {
		panic("deadlocals requires a compiler graph")
	}
	c := newContext()
	c.IR = graph
	return c
}

func (c *Context) checkFuncs(fns []*ir.Func) {
	for _, fn := range fns {
		if fn == nil || !c.IR.OwnsNode(fn) {
			panic("deadlocals function belongs to another compilation")
		}
	}
}

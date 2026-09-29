// Package staticdata exposes native static initializers, string data and
// deferred function-value symbols. File-backed embed data is not yet ported.
package staticdata

import (
	compiler "go-backend-ssa/internal/staticdata"
	"go-backend-ssa/ir"
	"go-backend-ssa/objw"
)

type Context = compiler.Context

// NewContext binds the existing IR graph and object writer. Symbol arguments
// must belong to that writer's Link. Serialize writes; see Context for the
// narrower concurrent function-value registration contract.
func NewContext(graph *ir.Context, writer *objw.Context) *Context {
	return compiler.NewContext(graph, writer)
}

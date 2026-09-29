// Package deadlocals exposes the native Go compiler's removal of assignments
// to unused local variables. Run Funcs after ABI wrappers and before escape
// analysis, using the same IR graph as the preceding compilation phases.
package deadlocals

import (
	compiler "go-backend-ssa/internal/deadlocals"
	"go-backend-ssa/ir"
)

type Context = compiler.Context

func NewContext(graph *ir.Context) *Context { return compiler.NewContext(graph) }

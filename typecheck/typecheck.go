// Package typecheck exposes the migrated native compiler's IR typecheck state.
// Context checks and rewrites native IR expressions and statements, initializes
// builtins/runtime declarations, and owns function declaration state. Source
// decoding through the noder reader is not yet integrated.
package typecheck

import (
	compiler "go-backend-ssa/internal/typecheck"
	"go-backend-ssa/ir"
	"go-backend-ssa/types"
)

type Context = compiler.Context
type ClosureStructIter = compiler.ClosureStructIter

// NewContext binds typecheck to an existing native IR instance. Call
// InitUniverse once instead of initializing compiler types separately.
func NewContext(graph *ir.Context) *Context { return compiler.NewContext(graph) }

// RangeExprType returns the native effective type of a range operand.
func RangeExprType(t *types.Type) *types.Type { return compiler.RangeExprType(t) }

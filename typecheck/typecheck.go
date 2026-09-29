// Package typecheck exposes the migrated native compiler's IR typecheck state.
// This currently initializes its universe and selected construction helpers;
// expression/statement checking and the noder reader are not yet integrated.
package typecheck

import (
	compiler "go-backend-ssa/internal/typecheck"
	"go-backend-ssa/ir"
)

type Context = compiler.Context

// NewContext binds typecheck to an existing native IR instance. Call
// InitUniverse once instead of initializing compiler types separately.
func NewContext(graph *ir.Context) *Context { return compiler.NewContext(graph) }

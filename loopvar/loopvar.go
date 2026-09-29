// Package loopvar exposes the native Go compiler's per-iteration variable
// capture transformation. Run it after inlining and method wrapper generation,
// before escape analysis. LogTransformations consumes final escape classifications.
package loopvar

import (
	compiler "go-backend-ssa/internal/loopvar"
	"go-backend-ssa/logopt"
	"go-backend-ssa/typecheck"
)

type Context = compiler.Context
type VarAndLoop = compiler.VarAndLoop

func NewContext(tc *typecheck.Context, log *logopt.Context) *Context {
	return compiler.NewContext(tc, log)
}

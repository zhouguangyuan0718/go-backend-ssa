// Package escape exposes the native Go compiler escape analysis. Run Funcs
// once after deadlocals, using the same typecheck graph and optimizer log.
package escape

import (
	compiler "go-backend-ssa/internal/escape"
	"go-backend-ssa/logopt"
	"go-backend-ssa/typecheck"
)

type Context = compiler.Context

func NewContext(tc *typecheck.Context, log *logopt.Context) *Context {
	return compiler.NewContext(tc, log)
}

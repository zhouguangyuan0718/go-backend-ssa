// Package staticinit exposes the native compiler's static-initialization
// scheduler and map initializer outlining on an existing compiler graph.
package staticinit

import (
	compiler "go-backend-ssa/internal/staticinit"
	"go-backend-ssa/ir"
	"go-backend-ssa/reflectdata"
	"go-backend-ssa/typecheck"
)

type Context = compiler.Context
type Schedule = compiler.Schedule
type Plan = compiler.Plan
type Entry = compiler.Entry

func NewContext(tc *typecheck.Context, rd *reflectdata.Context) *Context {
	return compiler.NewContext(tc, rd)
}

func AnySideEffects(n ir.Node) bool { return compiler.AnySideEffects(n) }

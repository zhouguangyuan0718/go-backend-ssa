// Package bloop exposes the native Go compiler B.Loop liveness pass.
package bloop

import (
	compiler "go-backend-ssa/internal/bloop"
	"go-backend-ssa/typecheck"
)

type Context = compiler.Context

func NewContext(tc *typecheck.Context) *Context { return compiler.NewContext(tc) }

// Package pgoir exposes native profile attribution on an existing compiler IR
// graph. Frontend function lookup and deferred-body cleanup must be bound on
// each Context before attributing a nonempty profile.
package pgoir

import (
	compiler "go-backend-ssa/internal/pgoir"
	"go-backend-ssa/typecheck"
)

type Context = compiler.Context
type Profile = compiler.Profile
type IRGraph = compiler.IRGraph
type IRNode = compiler.IRNode
type IREdge = compiler.IREdge
type CallSiteInfo = compiler.CallSiteInfo

func NewContext(tc *typecheck.Context) *Context { return compiler.NewContext(tc) }

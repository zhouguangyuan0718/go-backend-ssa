// Package ssagen currently exposes the native SymABIs and ABI wrapper phase.
// Run GenABIWrappers after pkginit.MakeTask. Native SSA construction is not yet
// exposed; the returned wrapper functions are native IR in the shared target.
package ssagen

import (
	compiler "go-backend-ssa/internal/ssagen"
	"go-backend-ssa/typecheck"
)

type Context = compiler.Context
type SymABIs = compiler.SymABIs

func NewContext(tc *typecheck.Context) *Context { return compiler.NewContext(tc) }

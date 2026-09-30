// Package ssagen exposes native ABI wrappers, SSA target initialization and
// frontend services for declared IR functions. Run GenABIWrappers after
// pkginit.MakeTask. Native IR-to-SSA construction is not yet exposed.
package ssagen

import (
	compiler "go-backend-ssa/internal/ssagen"
	"go-backend-ssa/typecheck"
)

type Context = compiler.Context
type SymABIs = compiler.SymABIs
type ArchInfo = compiler.ArchInfo

func NewContext(tc *typecheck.Context) *Context { return compiler.NewContext(tc) }

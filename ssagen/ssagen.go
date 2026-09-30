// Package ssagen exposes native ABI wrappers, SSA target initialization and
// frontend services for declared IR functions. Run GenABIWrappers after
// pkginit.MakeTask. Configure SSA and liveness services, then run InitConfig and
// InitTables before Walk and BuildSSA. BuildSSA includes phi placement, SSA
// optimization, register allocation, frame layout and defer metadata. Install
// target handlers before InitConfig; GenSSA then emits instructions and GC/debug
// metadata into an owned objw.Progs buffer. Consume results before worker reuse.
package ssagen

import (
	compiler "go-backend-ssa/internal/ssagen"
	"go-backend-ssa/typecheck"
)

type Context = compiler.Context
type SymABIs = compiler.SymABIs
type ArchInfo = compiler.ArchInfo

func NewContext(tc *typecheck.Context) *Context { return compiler.NewContext(tc) }

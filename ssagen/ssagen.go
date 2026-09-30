// Package ssagen exposes native ABI wrappers, SSA target initialization and
// frontend services for declared IR functions. Run GenABIWrappers after
// pkginit.MakeTask. Configure SSA and liveness services, then run InitConfig and
// InitTables before Walk and BuildSSA. BuildSSA includes phi placement, SSA
// optimization, register allocation, frame layout and defer metadata. Install
// target handlers before InitConfig; GenSSA then emits instructions and GC/debug
// metadata into an owned objw.Progs buffer. Compile performs the full native
// build, emission and Flush sequence, including field/map relocations and large
// frame collection. Bind Inline to the same compiler graph before Compile and
// call CheckLargeStacks after workers complete. Consume BuildSSA graphs before
// worker-cache reuse. ConstructSSA instead returns an independently cached graph
// after phi insertion, before SSA Compile; ContinueNative resumes the original
// pipeline once. All operations sharing one Context must be serialized.
package ssagen

import (
	compiler "go-backend-ssa/internal/ssagen"
	"go-backend-ssa/typecheck"
)

type Context = compiler.Context
type Construction = compiler.Construction
type OpenDeferLayout = compiler.OpenDeferLayout
type SymABIs = compiler.SymABIs
type ArchInfo = compiler.ArchInfo

func NewContext(tc *typecheck.Context) *Context { return compiler.NewContext(tc) }

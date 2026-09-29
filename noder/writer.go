package noder

import (
	"go-backend-ssa/gcbridge"
	compiler "go-backend-ssa/internal/noder"
	"go-backend-ssa/types"
	"go-backend-ssa/types2"
)

// Context binds the existing native checker, compiler types and Link to the
// original noder writer. One Context is used serially; independent Contexts
// support concurrent and nested calls without process-wide compiler state.
// WritePackage consumes checked/prepared ASTs and returns the native package
// stub bitstream. This is not yet a compiler IR or SSA construction entry point.
type Context = compiler.Context

func NewContext(checker *types2.Context, compilerTypes *types.Universe, link *gcbridge.Link) (*Context, error) {
	return compiler.NewContext(checker, compilerTypes, link)
}

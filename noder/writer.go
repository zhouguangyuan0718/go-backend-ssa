package noder

import (
	compiler "go-backend-ssa/internal/noder"
	"go-backend-ssa/typecheck"
	"go-backend-ssa/types2"
)

// Context binds the source checker and existing IR/typecheck graph to the
// original noder writer and migrated reader algorithms. One Context is used serially; independent Contexts
// support concurrent and nested calls without process-wide compiler state.
// WritePackage consumes checked/prepared ASTs and returns the native package
// stub bitstream. This is not yet a compiler IR or SSA construction entry point.
type Context = compiler.Context

// NewContext requires an initialized native compiler universe and shared Link
// position table. Configure flags/debug on tc.IR; this binding preserves them.
func NewContext(checker *types2.Context, tc *typecheck.Context) (*Context, error) {
	return compiler.NewContext(checker, tc)
}

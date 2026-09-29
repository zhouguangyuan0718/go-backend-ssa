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
// stub bitstream. RegisterPackage and ReadLocalPackage expose the native reader
// stages and return the existing package IR. Inline is bound to the native
// function-body reader for call-site expansion. Run Interleaved on the package,
// then MakeWrappers once to generate and optimize native method wrappers.
// The complete compiler pipeline through SSA construction is not yet integrated.
type Context = compiler.Context

// NewContext requires an initialized native compiler universe and shared Link
// position table. Configure flags/debug on tc.IR; this binding preserves them.
func NewContext(checker *types2.Context, tc *typecheck.Context) (*Context, error) {
	return compiler.NewContext(checker, tc)
}

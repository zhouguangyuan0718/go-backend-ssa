// Package ssa exposes the currently migrated native SSA operations.
// CanSSA is the type representation predicate; SSA graph construction is not
// yet available.
package ssa

import (
	compiler "go-backend-ssa/internal/ssa"
	"go-backend-ssa/types"
)

const MaxStruct = compiler.MaxStruct

// CanSSA reports whether values of t can be represented as an SSA Value.
// It uses t's owning compilation and computes its native layout if needed.
// As with other layout operations, callers serialize access to the same graph.
func CanSSA(t *types.Type) bool { return compiler.CanSSA(t) }

// Package ssa exposes the currently migrated native SSA operations.
// Native type collections, locations and the Frontend contract are available;
// SSA graph construction is not yet available.
package ssa

import (
	compiler "go-backend-ssa/internal/ssa"
	"go-backend-ssa/types"
)

const MaxStruct = compiler.MaxStruct

type Types = compiler.Types
type Logger = compiler.Logger
type Frontend = compiler.Frontend
type Location = compiler.Location
type Register = compiler.Register
type LocalSlot = compiler.LocalSlot
type LocPair = compiler.LocPair
type LocResults = compiler.LocResults
type Spill = compiler.Spill
type ID = compiler.ID

// NewTypes binds native SSA types to u. Initialize u with typecheck.InitUniverse
// first. Copies of the returned Types retain the same universe; SetTypPtrs
// restores their canonical type pointers within that universe.
func NewTypes(u *types.Universe) *Types { return compiler.NewTypes(u) }

// CanSSA reports whether values of t can be represented as an SSA Value.
// It uses t's owning compilation and computes its native layout if needed.
// As with other layout operations, callers serialize access to the same graph.
func CanSSA(t *types.Type) bool { return compiler.CanSSA(t) }

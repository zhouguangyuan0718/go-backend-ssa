// Package ssa exposes the currently migrated native SSA operations.
// Native graph construction and target configuration are available. The complete
// compiler pipeline is still being migrated.
package ssa

import (
	"go-backend-ssa/abi"
	"go-backend-ssa/internal/go/buildcfg"
	compiler "go-backend-ssa/internal/ssa"
	"go-backend-ssa/logopt"
	"go-backend-ssa/reflectdata"
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
type Op = compiler.Op
type BlockKind = compiler.BlockKind
type SymEffect = compiler.SymEffect

const (
	SymRead  = compiler.SymRead
	SymWrite = compiler.SymWrite
	SymAddr  = compiler.SymAddr
	SymRdWr  = compiler.SymRdWr
	SymNone  = compiler.SymNone
)

// NewTypes binds native SSA types to u. Initialize u with typecheck.InitUniverse
// first. Copies of the returned Types retain the same universe; SetTypPtrs
// restores their canonical type pointers within that universe.
func NewTypes(u *types.Universe) *Types { return compiler.NewTypes(u) }

// CanSSA reports whether values of t can be represented as an SSA Value.
// It uses t's owning compilation and computes its native layout if needed.
// As with other layout operations, callers serialize access to the same graph.
func CanSSA(t *types.Type) bool { return compiler.CanSSA(t) }

// Context owns target rewrite callbacks and opcode tables for one compilation.
type Context = compiler.Context
type Config = compiler.Config
type Func = compiler.Func
type FuncDebug = compiler.FuncDebug
type Block = compiler.Block
type Value = compiler.Value
type Cache = compiler.Cache
type Edge = compiler.Edge
type GoarmFeatures = buildcfg.GoarmFeatures

// NewContext uses the compiler graph already shared by ABI and reflection data.
// Set the target's feature fields explicitly before calling Context.NewConfig.
func NewContext(a *abi.Context, rd *reflectdata.Context, log *logopt.Context) *Context {
	return compiler.NewContext(a, rd, log)
}

// Compile runs the complete native SSA pipeline on f. Configure the owning
// Context's clock and any enabled metric or dump providers before calling it.
func Compile(f *Func) { compiler.Compile(f) }

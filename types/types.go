// Package types exposes compiler object identity and compilation ownership.
// Initialize builtin types with Universe.InitTypes before computing layouts.
// Parsing, type checking and source-to-SSA construction are not yet available.
package types

import (
	"fmt"
	compiler "go-backend-ssa/internal/types"
	"go-backend-ssa/position"
	"go-backend-ssa/target"
)

type Universe = compiler.Universe
type Type = compiler.Type
type Kind = compiler.Kind
type Pkg = compiler.Pkg
type Sym = compiler.Sym
type Object = compiler.Object
type ChanDir = compiler.ChanDir
type Map = compiler.Map
type Field = compiler.Field
type Struct = compiler.Struct
type AlgKind = compiler.AlgKind

const (
	Int8          = compiler.TINT8
	Uint8         = compiler.TUINT8
	Int16         = compiler.TINT16
	Uint16        = compiler.TUINT16
	Int32         = compiler.TINT32
	Uint32        = compiler.TUINT32
	Int64         = compiler.TINT64
	Uint64        = compiler.TUINT64
	Int           = compiler.TINT
	Uint          = compiler.TUINT
	Uintptr       = compiler.TUINTPTR
	Complex64     = compiler.TCOMPLEX64
	Complex128    = compiler.TCOMPLEX128
	Float32       = compiler.TFLOAT32
	Float64       = compiler.TFLOAT64
	Bool          = compiler.TBOOL
	String        = compiler.TSTRING
	UnsafePointer = compiler.TUNSAFEPTR
	Tuple         = compiler.TTUPLE
	Results       = compiler.TRESULTS
	SSA           = compiler.TSSA
	Pointer       = compiler.TPTR
	Slice         = compiler.TSLICE
	Array         = compiler.TARRAY
	MapKind       = compiler.TMAP
	Chan          = compiler.TCHAN
	StructKind    = compiler.TSTRUCT
	Func          = compiler.TFUNC
	Interface     = compiler.TINTER
	RecvOnly      = compiler.Crecv
	SendOnly      = compiler.Csend
	SendRecv      = compiler.Cboth
)

func NewUniverse(layout target.Layout) (*Universe, error) {
	if !layout.Valid() {
		return nil, fmt.Errorf("explicit target layout required")
	}
	return compiler.NewUniverse(layout.PointerSize(), layout.RegisterSize(), layout.MaxWidth()), nil
}
func CompareSyms(a, b *Sym) int { return compiler.CompareSyms(a, b) }

// Original compiler comparison/hash algorithm categories.
const (
	AUNK      = compiler.AUNK
	ANOEQ     = compiler.ANOEQ
	ANOALG    = compiler.ANOALG
	AMEM      = compiler.AMEM
	AMEM0     = compiler.AMEM0
	AMEM8     = compiler.AMEM8
	AMEM16    = compiler.AMEM16
	AMEM32    = compiler.AMEM32
	AMEM64    = compiler.AMEM64
	AMEM128   = compiler.AMEM128
	ASTRING   = compiler.ASTRING
	AINTER    = compiler.AINTER
	ANILINTER = compiler.ANILINTER
	AFLOAT32  = compiler.AFLOAT32
	AFLOAT64  = compiler.AFLOAT64
	ACPLX64   = compiler.ACPLX64
	ACPLX128  = compiler.ACPLX128
	ASPECIAL  = compiler.ASPECIAL
)

// NewUniverseWithPositions shares an existing native position namespace from
// construction onward. Pass &link.PosTable when integrating native IR or noder.
func NewUniverseWithPositions(layout target.Layout, positions *position.Table) (*Universe, error) {
	if !layout.Valid() || positions == nil {
		return nil, fmt.Errorf("explicit target layout and position table required")
	}
	return compiler.NewUniverseWithPositions(layout.PointerSize(), layout.RegisterSize(), layout.MaxWidth(), positions), nil
}

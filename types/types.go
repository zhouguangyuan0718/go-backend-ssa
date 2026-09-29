// Package types exposes compiler object identity and compilation ownership.
// This is a partial frontend model: layout calculation, builtin bindings and
// type checking are not yet implemented. Scalars have uncomputed width/alignment.
package types

import (
	"fmt"
	compiler "go-backend-ssa/internal/types"
	"go-backend-ssa/target"
)

type Universe = compiler.Universe
type Type = compiler.Type
type Kind = compiler.Kind
type Pkg = compiler.Pkg
type Sym = compiler.Sym
type Object = compiler.Object

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
)

func NewUniverse(layout target.Layout) (*Universe, error) {
	if !layout.Valid() {
		return nil, fmt.Errorf("explicit target layout required")
	}
	return compiler.NewUniverse(layout.PointerSize(), layout.RegisterSize(), layout.MaxWidth()), nil
}
func CompareSyms(a, b *Sym) int { return compiler.CompareSyms(a, b) }

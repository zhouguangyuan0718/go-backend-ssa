// Library-owned compilation state. Compiler object definitions and algorithms
// are generated from upstream; no placeholder Type, Sym or linker symbol is used.
package types

import (
	"fmt"
	"sync"
)

// Universe owns one compilation's mutable type and package graph. Its zero value
// is invalid. Except for InternString, calls within a Universe are serialized.
type Universe struct {
	mu               sync.Mutex
	strings          map[string]string
	packages         map[string]*Pkg
	anonymous        *Pkg
	ptrSize, regSize int
	maxWidth         int64
	// Upstream SSA singleton names are retained as per-compilation fields.
	// Treat these bindings as read-only after construction, like the upstream
	// globals; Type flags still belong to this Universe and may be mutated.
	TypeInvalid, TypeMem, TypeFlags, TypeVoid                *Type
	TypeInt128, TypeVec128, TypeVec256, TypeVec512, TypeMask *Type
	TypeResultMem                                            *Type
	NewPtrCacheEnabled                                       bool
	scalars                                                  [NTYPE]*Type
}

func NewUniverse(ptrSize, regSize int, maxWidth int64) *Universe {
	if (ptrSize != 4 && ptrSize != 8) || regSize != ptrSize || maxWidth <= 0 {
		panic("invalid target layout")
	}
	u := newUniverse()
	u.ptrSize, u.regSize, u.maxWidth = ptrSize, regSize, maxWidth
	return u
}
func (u *Universe) PointerSize() int  { return u.ptrSize }
func (u *Universe) RegisterSize() int { return u.regSize }
func (u *Universe) MaxWidth() int64   { return u.maxWidth }

// Scalar returns an interned, unnamed scalar node. Width/alignment and builtin
// declaration bindings are not initialized until the later layout/noding phase.
func (u *Universe) Scalar(kind Kind) (*Type, error) {
	if u == nil || u.ptrSize == 0 {
		return nil, fmt.Errorf("uninitialized Universe")
	}
	if !(kind >= TINT8 && kind <= TBOOL || kind == TSTRING || kind == TUNSAFEPTR) {
		return nil, fmt.Errorf("not a supported scalar kind: %d", kind)
	}
	if t := u.scalars[kind]; t != nil {
		return t, nil
	}
	t := u.newType(kind)
	u.scalars[kind] = t
	return t, nil
}

func (t *Type) Universe() *Universe {
	if t == nil {
		return nil
	}
	return t.universe
}
func (p *Pkg) Universe() *Universe {
	if p == nil {
		return nil
	}
	return p.universe
}
func (s *Sym) Universe() *Universe {
	if s == nil {
		return nil
	}
	return s.Pkg.Universe()
}
func (t *Type) TupleTypes() (*Type, *Type, error) {
	if t == nil || t.kind != TTUPLE {
		return nil, nil, fmt.Errorf("not a tuple")
	}
	pair := t.extra.(*Tuple)
	return pair.first, pair.second, nil
}

// Lookup explicitly supplies the owner for the upstream nil-package case.
func (u *Universe) Lookup(pkg *Pkg, name string) (*Sym, error) {
	if u == nil || u.ptrSize == 0 {
		return nil, fmt.Errorf("uninitialized Universe")
	}
	if pkg == nil {
		pkg = u.anonymous
	}
	if pkg.universe != u {
		return nil, fmt.Errorf("package belongs to another Universe")
	}
	return pkg.Lookup(name), nil
}

// A violated compiler invariant aborts this call, never the embedding process.
// This binds the original base.Fatalf signature; it is not a source diagnostic.
func (u *Universe) fatalf(format string, args ...any) { panic(fmt.Errorf(format, args...)) }

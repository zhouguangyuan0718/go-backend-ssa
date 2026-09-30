package ssa

import "go-backend-ssa/internal/types"

// This adapter binds native SSA type operations to their compilation's universe.
// It introduces no separate public session or process-global state.
type typeContext struct {
	Types   *types.Universe
	ptrSize int
}

func typeContextOf(t *types.Type) *typeContext {
	if t == nil || t.Universe() == nil {
		panic("SSA type predicate requires an initialized compiler type")
	}
	u := t.Universe()
	return &typeContext{Types: u, ptrSize: u.PointerSize()}
}

// NewTypes populates the native SSA type collection from an initialized universe.
// Calls that share a universe must be serialized, like other type operations.
func NewTypes(u *types.Universe) *Types {
	c := &typeContext{Types: u}
	return c.NewTypes()
}

func (c *typeContext) checkTypes() {
	if c.Types == nil {
		panic("SSA types require an initialized universe")
	}
	// Validate before the native initializer writes fields or interns pointers.
	// The public universe fields can be modified by callers.
	for _, k := range []types.Kind{types.TBOOL, types.TINT8, types.TINT16, types.TINT32, types.TINT64,
		types.TUINT8, types.TUINT16, types.TUINT32, types.TUINT64, types.TINT,
		types.TFLOAT32, types.TFLOAT64, types.TUINT, types.TUINTPTR, types.TSTRING} {
		t := c.Types.Types[k]
		if t == nil || t.Universe() != c.Types || t.Kind() != k {
			panic("SSA builtin type belongs to another or uninitialized universe")
		}
	}
	for _, t := range []*types.Type{c.Types.TypeVec128, c.Types.TypeVec256, c.Types.TypeVec512, c.Types.TypeMask} {
		if t == nil || t.Universe() != c.Types {
			panic("SSA vector type belongs to another or uninitialized universe")
		}
	}
}

package ssa

import "go-backend-ssa/internal/types"

// This adapter derives target inputs from the native type. It has no mutable
// SSA state and requires no separate session from the caller.
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

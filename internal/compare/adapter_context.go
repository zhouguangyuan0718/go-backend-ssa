package compare

import "go-backend-ssa/internal/types"

// The native predicate gets its existing compiler universe from the input.
// There is no additional mutable comparison state.
type typeContext struct{ Types *types.Universe }

func typeContextOf(t *types.Type) *typeContext {
	if t == nil || t.Universe() == nil {
		panic("comparison requires an initialized compiler type")
	}
	return &typeContext{Types: t.Universe()}
}

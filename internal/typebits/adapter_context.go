package typebits

import (
	"fmt"
	"go-backend-ssa/internal/types"
)

// The target is already owned by the input Type. Derive it for each native
// entry rather than introducing another mutable compiler context or global.
type typeContext struct{ ptrSize int }

func typeContextOf(t *types.Type) *typeContext {
	if t == nil || t.Universe() == nil {
		panic("type bitmap requires an initialized compiler type")
	}
	return &typeContext{ptrSize: t.Universe().PointerSize()}
}
func (*typeContext) fatalf(f string, a ...any) { panic(fmt.Errorf(f, a...)) }

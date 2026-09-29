package bitvec

import "fmt"

// BitVec carries no compiler state. Only the native fatal boundary changes;
// keep the original value layout and method signatures.
type fatalContext struct{}

func bitvecFatal(BitVec) *fatalContext          { return &fatalContext{} }
func (*fatalContext) fatalf(f string, a ...any) { panic(fmt.Errorf(f, a...)) }

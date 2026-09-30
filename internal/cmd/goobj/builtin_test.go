package goobj

import (
	"fmt"
	"testing"
)

func TestBuiltinContexts(t *testing.T) {
	for n := range 8 {
		t.Run(fmt.Sprint(n), func(t *testing.T) {
			t.Parallel()
			a, b := NewContext(true), NewContext(false)
			if NBuiltin() != len(a.builtins) {
				t.Fatal("builtin table length")
			}
			for i := 0; i < NBuiltin(); i++ {
				name, abi := a.BuiltinName(i)
				if a.BuiltinIdx(name, abi) != i || a.BuiltinIdx(name, abi^1) != -1 || b.BuiltinIdx(name, abi^1) != i {
					t.Fatal("native ABI gate", i)
				}
			}
			name, _ := a.BuiltinName(0)
			delete(b.builtinMap, name)
			b.builtins[0].name = "changed"
			if a.BuiltinIdx(name, a.builtins[0].abi) != 0 || a.builtins[0].name != name {
				t.Fatal("builtin tables shared")
			}
		})
	}
}

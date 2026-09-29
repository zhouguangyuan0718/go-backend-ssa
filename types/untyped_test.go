package types_test

import (
	"go-backend-ssa/types"
	"go/constant"
	"sync"
	"testing"
)

func TestUntypedIdentityAndClassification(t *testing.T) {
	a, b := universe(t, "amd64"), universe(t, "386")
	for _, u := range []*types.Universe{a, b} {
		entries := []struct {
			kind                       constant.Kind
			typ                        *types.Type
			defaultKind                types.Kind
			integer, floating, complex bool
		}{
			{constant.Bool, u.UntypedBool, types.Bool, false, false, false},
			{constant.String, u.UntypedString, types.String, false, false, false},
			{constant.Int, u.UntypedInt, types.Int, true, false, false},
			{constant.Float, u.UntypedFloat, types.Float64, false, true, false},
			{constant.Complex, u.UntypedComplex, types.Complex128, false, false, true},
		}
		for _, e := range entries {
			typ := u.UntypedTypes[e.kind]
			if typ != e.typ || typ.Universe() != u || !typ.IsUntyped() || typ.IsInteger() != e.integer || typ.IsFloat() != e.floating || typ.IsComplex() != e.complex || u.DefaultKinds[e.kind] != e.defaultKind {
				t.Fatal("untyped mapping/classification changed", e.kind)
			}
		}
		if !u.UntypedRune.IsInteger() || !u.UntypedRune.IsUntyped() || u.UntypedRune == u.UntypedInt || !u.UntypedString.IsString() || !u.UntypedBool.IsBoolean() {
			t.Fatal("alias or string/bool identity lost")
		}
		for _, kind := range []types.Kind{types.Int, types.Float64, types.Complex128, types.String, types.Bool} {
			typ, _ := u.Scalar(kind)
			if typ.IsUntyped() || !typ.IsKind(kind) {
				t.Fatal("typed scalar confused with untyped constant", kind)
			}
		}
	}
	var absent *types.Type
	if absent.IsUntyped() || absent.IsKind(types.Int) {
		t.Fatal("upstream nil behavior changed")
	}
	if (*types.Type).IsUntyped(nil) {
		t.Fatal("method expression nil behavior changed")
	}
	for kind := constant.Bool; kind <= constant.Complex; kind++ {
		if a.UntypedTypes[kind] == b.UntypedTypes[kind] {
			t.Fatal("shared constant type", kind)
		}
	}
	callback := a.UntypedInt.IsInteger
	a.UntypedTypes[constant.Int] = a.UntypedRune
	a.DefaultKinds[constant.Int] = types.Uint
	if b.UntypedTypes[constant.Int] != b.UntypedInt || b.DefaultKinds[constant.Int] != types.Int || !callback() {
		t.Fatal("constant tables or callback lost owner")
	}
	for i := 0; i < 30; i++ {
		u := universe(t, "arm64")
		u.UntypedString.SetNoalg(true)
	}
	if !a.UntypedString.IsUntyped() || a.UntypedString.Noalg() {
		t.Fatal("later instance invalidated constant types")
	}
}

func TestConcurrentUntypedUniverses(t *testing.T) {
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			u := universe(t, "amd64")
			for j := 0; j < 40; j++ {
				u.UntypedRune.SetHasShape(j%2 == 0)
				u.UntypedTypes[constant.Int] = u.UntypedRune
				if !u.UntypedRune.IsInteger() || !u.UntypedRune.IsUntyped() || u.UntypedInt.HasShape() || u.UntypedTypes[constant.Int].Universe() != u {
					t.Error("untyped state crossed instances")
					return
				}
			}
		}()
	}
	wg.Wait()
}

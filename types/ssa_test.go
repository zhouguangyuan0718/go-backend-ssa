package types_test

import (
	"go-backend-ssa/types"
	"sync"
	"testing"
)

func TestSSATypeIdentityAndMemoryResults(t *testing.T) {
	a, b := universe(t, "amd64"), universe(t, "386")
	states := func(u *types.Universe) []*types.Type {
		return []*types.Type{u.TypeInvalid, u.TypeMem, u.TypeFlags, u.TypeVoid, u.TypeInt128, u.TypeVec128, u.TypeVec256, u.TypeVec512, u.TypeMask, u.TypeResultMem}
	}
	for i, x := range states(a) {
		y := states(b)[i]
		if x == nil || x == y || x.Universe() != a || y.Universe() != b || x.Underlying() != x {
			t.Fatal("global singleton survived", i)
		}
		x.SetNoalg(true)
		if y.Noalg() {
			t.Fatal("shared SSA flags", i)
		}
	}
	value, _ := a.Scalar(types.Int)
	cases := []struct {
		name   string
		typ    *types.Type
		memory bool
	}{
		{"memory", a.TypeMem, true}, {"flags", a.TypeFlags, false}, {"void", a.TypeVoid, false},
		{"scalar", value, false}, {"tuple trailing memory", a.NewTuple(value, a.TypeMem), true},
		{"tuple leading memory", a.NewTuple(a.TypeMem, value), false},
		{"empty results", a.NewResults(nil), false},
		{"trailing memory result", a.NewResults([]*types.Type{value, a.TypeMem}), true},
		{"leading memory result", a.NewResults([]*types.Type{a.TypeMem, value}), false},
	}
	for _, c := range cases {
		if c.typ.IsMemory() != c.memory || c.typ.Universe() != a {
			t.Error(c.name)
		}
	}
	if a.NewResults([]*types.Type{a.TypeMem}) != a.TypeResultMem || !a.TypeResultMem.IsResults() || !a.TypeResultMem.IsMemory() {
		t.Fatal("upstream result-memory cache lost")
	}
	if !a.TypeFlags.IsFlags() || !a.TypeVoid.IsVoid() || a.TypeMem.IsFlags() || a.TypeInt128.IsSIMD() {
		t.Fatal("SSA classification changed")
	}
	for _, x := range []*types.Type{a.TypeVec128, a.TypeVec256, a.TypeVec512, a.TypeMask} {
		if !x.IsSIMD() {
			t.Fatal("lost SIMD marker")
		}
	}
	makeResults := a.NewResults
	expr := (*types.Universe).NewTuple
	old := expr(a, value, a.TypeMem)
	for i := 0; i < 30; i++ {
		u := universe(t, "arm64")
		u.TypeMem.SetHasShape(true)
		u.NewResults([]*types.Type{u.TypeMem})
	}
	if makeResults([]*types.Type{a.TypeMem}) != a.TypeResultMem || !old.IsMemory() || a.TypeMem.HasShape() {
		t.Fatal("later instance invalidated old graph")
	}
	for name, f := range map[string]func(){
		"foreign tuple":              func() { a.NewTuple(value, b.TypeMem) },
		"foreign results":            func() { a.NewResults([]*types.Type{b.TypeMem}) },
		"nil result":                 func() { a.NewResults([]*types.Type{nil}) },
		"nil tuple":                  func() { a.NewTuple(nil, a.TypeMem) },
		"unowned":                    func() { a.NewTuple(new(types.Type), a.TypeMem) },
		"nil universe empty results": func() { (*types.Universe)(nil).NewResults(nil) },
	} {
		t.Run(name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Error("invalid construction accepted")
				}
			}()
			f()
		})
	}
}

func TestConcurrentSSAUniverses(t *testing.T) {
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			u := universe(t, []string{"amd64", "arm64", "386"}[i%3])
			for j := 0; j < 60; j++ {
				u.TypeMem.SetHasShape(j%2 == 0)
				tuple := u.NewTuple(u.TypeInt128, u.TypeMem)
				results := u.NewResults([]*types.Type{u.TypeMem})
				if !tuple.IsMemory() || tuple.HasShape() != u.TypeMem.HasShape() || results != u.TypeResultMem || results.Universe() != u {
					t.Error("shared SSA state")
					return
				}
			}
		}(i)
	}
	wg.Wait()
}

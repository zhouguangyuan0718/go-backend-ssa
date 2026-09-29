package types

import "testing"

func TestPerUniverseUpstreamSSAInitialization(t *testing.T) {
	for _, size := range []int{4, 8} {
		u := NewUniverse(size, size, 1<<50)
		for _, c := range []struct {
			typ   *Type
			width int64
			simd  bool
		}{
			{u.TypeInt128, 16, false}, {u.TypeVec128, 16, true}, {u.TypeVec256, 32, true}, {u.TypeVec512, 64, true}, {u.TypeMask, 8, true},
		} {
			if c.typ.width != c.width || c.typ.align != 8 || c.typ.isSIMD != c.simd || c.typ.universe != u {
				t.Fatalf("upstream init not applied for target %d: %+v", size, c)
			}
		}
		if u.TypeMem.width != BADWIDTH || u.TypeMem.align != 0 {
			t.Fatal("invented memory layout")
		}
		if got := u.TypeResultMem.extra.(*Results).Types; len(got) != 1 || got[0] != u.TypeMem {
			t.Fatal("initializer dependencies lost")
		}
		input := []*Type{u.TypeFlags, u.TypeMem}
		results := u.NewResults(input)
		if got := results.extra.(*Results).Types; &got[0] != &input[0] {
			t.Fatal("upstream result slice ownership changed")
		}
	}
}

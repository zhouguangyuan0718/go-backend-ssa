package ssa

import (
	"go-backend-ssa/internal/types"
	"testing"
)

func TestNativeSSATypeComparison(t *testing.T) {
	u, _ := auxFixture(8)
	v, _ := auxFixture(4)
	list := []*types.Type{nil, u.Types[types.TINT], u.Types[types.TUINT8], u.ByteType, u.RuneType, u.Types[types.TINT32], u.TypeMem, u.TypeFlags, u.TypeVoid, u.NewPtr(u.Types[types.TINT]), u.NewSlice(u.Types[types.TINT]), u.NewArray(u.Types[types.TINT], 0), u.NewArray(u.Types[types.TINT], 3), u.NewArray(u.Types[types.TINT], 3), u.NewMap(u.Types[types.TINT], u.Types[types.TSTRING]), u.NewInterface(nil), u.AnyType, u.NewTuple(u.Types[types.TINT], u.TypeMem), u.NewResults([]*types.Type{u.Types[types.TINT], u.TypeMem})}
	for i, a := range list {
		for j, b := range list {
			ab := a.Compare(b)
			ba := b.Compare(a)
			if ab < types.CMPlt || ab > types.CMPgt || ab != -ba || i == j && ab != types.CMPeq {
				t.Fatalf("ordering %d %d: %d %d", i, j, ab, ba)
			}
			for k, c := range list {
				if ab <= types.CMPeq && b.Compare(c) <= types.CMPeq && a.Compare(c) > types.CMPeq {
					t.Fatalf("transitivity %d %d %d", i, j, k)
				}
			}
		}
	}
	if u.ByteType.Compare(u.Types[types.TUINT8]) != types.CMPeq || u.RuneType.Compare(u.Types[types.TINT32]) != types.CMPeq || u.AnyType.Compare(u.NewInterface(nil)) != types.CMPeq {
		t.Fatal("native alias equivalence")
	}
	defer func() {
		if recover() == nil {
			t.Fatal("foreign universe compared")
		}
	}()
	u.Types[types.TINT].Compare(v.Types[types.TINT])
}

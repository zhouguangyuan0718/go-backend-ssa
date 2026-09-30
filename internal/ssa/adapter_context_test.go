package ssa

import (
	"fmt"
	"go-backend-ssa/internal/cmd/obj"
	"go-backend-ssa/internal/cmd/src"
	"go-backend-ssa/internal/ir"
	"go-backend-ssa/internal/typecheck"
	"go-backend-ssa/internal/types"
	"testing"
)

func universe(size int) *types.Universe {
	link := &obj.Link{}
	u := types.NewUniverseWithPositions(size, size, 1<<50, &link.PosTable)
	typecheck.NewContext(ir.NewContext(u, link)).InitUniverse()
	return u
}
func record(u *types.Universe, ts ...*types.Type) *types.Type {
	var fs []*types.Field
	for _, t := range ts {
		fs = append(fs, u.NewField(src.XPos{}, nil, t))
	}
	return u.NewStruct(fs)
}
func TestNativeCanSSA(t *testing.T) {
	for _, size := range []int{4, 8} {
		for _, simd := range []bool{false, true} {
			t.Run(fmt.Sprint(size, simd), func(t *testing.T) {
				u := universe(size)
				u.Experiment.SIMD = simd
				i := u.Types[types.TINT]
				z := u.NewArray(i, 0)
				p := u.NewPtr(i)
				for _, c := range []struct {
					name string
					t    *types.Type
					want bool
				}{
					{"int", i, true}, {"slice", u.NewSlice(i), true}, {"interface", u.NewInterface(nil), true},
					{"zero-array", z, true}, {"singleton", u.NewArray(i, 1), true}, {"array", u.NewArray(i, 2), false}, {"zero-sized-array", u.NewArray(z, 100), true},
					{"four-fields", record(u, i, i, i, i), true}, {"five-fields", record(u, i, i, i, i, i), false}, {"nested-array", record(u, u.NewArray(i, 2)), false},
					{"direct-interface-many-fields", record(u, z, z, z, z, p), true},
					{"pointer-size-limit", record(u, u.Types[types.TINT64], u.Types[types.TINT64], u.Types[types.TINT64]), size == 8},
					{"vector", u.TypeVec512, true}, {"vector-record", record(u, u.TypeVec512), simd}, {"two-vector-record", record(u, u.TypeVec512, u.TypeVec512), simd},
					{"expanded-limit", record(u, u.Types[types.TCOMPLEX128], u.Types[types.TCOMPLEX128]), size == 8 || simd},
					{"too-many-registers", record(u, u.Types[types.TCOMPLEX128], u.Types[types.TCOMPLEX128], u.Types[types.TCOMPLEX128]), false},
					{"four-vector-fields", record(u, u.TypeVec512, u.TypeVec512, u.TypeVec512, u.NewArray(u.TypeVec512, 1)), simd},
				} {
					if got := CanSSA(c.t); got != c.want {
						t.Errorf("%s: got %t want %t", c.name, got, c.want)
					}
				}
			})
		}
	}
}
func TestCanSSAIsolation(t *testing.T) {
	old := universe(4)
	a := record(old, old.Types[types.TINT64], old.Types[types.TINT64], old.Types[types.TINT64])
	experimentLimited := record(old, old.Types[types.TCOMPLEX128], old.Types[types.TCOMPLEX128])
	if CanSSA(a) || CanSSA(experimentLimited) {
		t.Fatal("32 bit and disabled SIMD limits")
	}
	for j := 0; j < 8; j++ {
		t.Run(fmt.Sprint(j), func(t *testing.T) {
			t.Parallel()
			u := universe(8)
			u.Experiment.SIMD = true
			b := record(u, u.TypeVec512)
			if !CanSSA(b) || CanSSA(a) || CanSSA(experimentLimited) {
				t.Fatal("type target isolation")
			}
		})
	}
}
func TestCanSSARejectsNil(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("nil input accepted")
		}
	}()
	CanSSA(nil)
}

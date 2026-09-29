package typebits

import (
	"go-backend-ssa/internal/bitvec"
	"go-backend-ssa/internal/cmd/obj"
	"go-backend-ssa/internal/cmd/src"
	"go-backend-ssa/internal/ir"
	"go-backend-ssa/internal/typecheck"
	"go-backend-ssa/internal/types"
	"sync"
	"testing"
)

func universe(size int) *types.Universe {
	link := &obj.Link{}
	u := types.NewUniverseWithPositions(size, size, 1<<50, &link.PosTable)
	typecheck.NewContext(ir.NewContext(u, link)).InitUniverse()
	return u
}
func TestTypeOwnsBitmapTarget(t *testing.T) {
	var wg sync.WaitGroup
	for i := range 16 {
		wg.Go(func() {
			size := 4 + 4*(i%2)
			u := universe(size)
			p := u.NewPtr(u.Types[types.TINT])
			typ := u.NewArray(p, 3)
			u.CalcSize(typ)
			bv := bitvec.New(5)
			Set(typ, int64(size), bv)
			if bv.Count() != 3 || bv.Next(0) != 1 || !bv.Get(3) {
				t.Error("target from type", bv)
			}
			nonheap := u.NewStruct([]*types.Field{u.NewField(src.XPos{}, nil, p)})
			nonheap.SetNotInHeap(true)
			excluded := u.NewPtr(nonheap)
			u.CalcSize(excluded)
			bv.Clear()
			Set(excluded, 0, bv)
			if !bv.IsEmpty() {
				t.Error("not-in-heap pointer scanned")
			}
			iface := u.NewInterface(nil)
			u.CalcSize(iface)
			Set(iface, 0, bv)
			if bv.Count() != 1 || !bv.Get(1) {
				t.Error("interface type word scanned")
			}
		})
	}
	wg.Wait()
}
func TestNativeAlignmentAndBoundsFailures(t *testing.T) {
	mustPanic := func(fn func()) {
		t.Helper()
		defer func() {
			if recover() == nil {
				t.Error("expected panic")
			}
		}()
		fn()
	}
	u := universe(8)
	p := u.NewPtr(u.Types[types.TINT])
	u.CalcSize(p)
	bv := bitvec.New(1)
	for _, fn := range []func(){func() { Set(p, 1, bv) }, func() { SetNoCheck(p, 1, bv) }, func() { Set(p, 8, bv) }, func() { Set(nil, 0, bv) }} {
		mustPanic(fn)
	}
	if !bv.IsEmpty() {
		t.Fatal("rejected single pointer changed bitmap")
	}
	// NoCheck skips the initial type alignment check, retaining pointer alignment
	// checks; a misaligned pointer-free scalar needs no bitmap entries.
	scalar := u.Types[types.TUINT64]
	mustPanic(func() { Set(scalar, 1, bv) })
	SetNoCheck(scalar, 1, bv)
	if !bv.IsEmpty() {
		t.Fatal("pointer-free scalar marked")
	}
}

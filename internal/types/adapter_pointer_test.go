package types

import "testing"

func TestPointerLayoutComesFromOwner(t *testing.T) {
	for _, size := range []int{4, 8} {
		u := NewUniverse(size, size, 1<<50)
		x, _ := u.Scalar(TINT)
		p := u.NewPtr(x)
		if p.width != int64(size) || p.align != uint8(size) || p.ptrBytes != int64(size) || p.intRegs != 1 || p.alg != AMEM {
			t.Fatalf("pointer layout for %d: %+v", size, p)
		}
		y, _ := u.Scalar(TBOOL)
		y.SetNoalg(true)
		q := u.NewPtr(y)
		if q.alg != ANOALG || !q.Noalg() {
			t.Fatal("Noalg propagation")
		}
		if slice := u.NewSlice(x); slice.width != BADWIDTH || slice.align != 0 {
			t.Fatal("uncomputed slice layout fabricated")
		}
	}
}

package bitvec

import (
	"slices"
	"testing"
)

func TestNativeBitVecBoundariesAndBulk(t *testing.T) {
	for _, n := range []int32{0, 1, 31, 32, 33, 65} {
		b := New(n)
		b.Not()
		if b.Count() != int(n) {
			t.Fatal("tail bits", n, b)
		}
		b.Not()
		if !b.IsEmpty() {
			t.Fatal("involution")
		}
		bulk := NewBulk(n, 2)
		a, c := bulk.Next(), bulk.Next()
		if n > 0 {
			a.Set(n - 1)
			if c.Get(n - 1) {
				t.Fatal("bulk overlap")
			}
			c.Copy(a)
			if !a.Eq(c) {
				t.Fatal("copy")
			}
		}
	}
	b := New(33)
	b.Set(32)
	saved := slices.Clone(b.B)
	for _, fn := range []func(){func() { b.Get(-1) }, func() { b.Set(33) }, func() { b.Unset(33) }, func() { b.Eq(New(32)) }} {
		func() {
			defer func() {
				if recover() == nil {
					t.Error("expected native bounds failure")
				}
			}()
			fn()
		}()
		if !slices.Equal(saved, b.B) {
			t.Fatal("failed call changed bits")
		}
	}
}

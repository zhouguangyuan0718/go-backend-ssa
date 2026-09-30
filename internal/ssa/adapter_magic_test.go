package ssa

import (
	"math/big"
	"testing"
)

// Check the documented multiplication/shift identities against division, using
// arbitrary precision only in this independent oracle (including 64-bit inputs).
func TestNativeMagicDivisionIdentities(t *testing.T) {
	checkUnsigned := func(n uint, d, x uint64) {
		if !umagicOK(n, int64(d)) {
			return
		}
		m := umagic(n, int64(d))
		factor := new(big.Int).Lsh(big.NewInt(1), n)
		factor.Add(factor, new(big.Int).SetUint64(m.m))
		got := new(big.Int).Mul(new(big.Int).SetUint64(x), factor)
		got.Rsh(got, n+uint(m.s))
		if got.Uint64() != x/d {
			t.Fatalf("unsigned %d: %d/%d got %s", n, x, d, got)
		}
	}
	checkSigned := func(n uint, d, x int64) {
		if !smagicOK(n, d) {
			return
		}
		m := smagic(n, d)
		got := new(big.Int).Mul(big.NewInt(x), new(big.Int).SetUint64(m.m))
		got.Rsh(got, n+uint(m.s))
		if x < 0 {
			got.Add(got, big.NewInt(1))
		}
		if got.Int64() != x/d {
			t.Fatalf("signed %d: %d/%d got %s", n, x, d, got)
		}
	}
	for d := uint64(1); d < 256; d++ {
		for x := uint64(0); x < 256; x++ {
			checkUnsigned(8, d, x)
		}
	}
	for d := int64(1); d < 128; d++ {
		for x := int64(-128); x < 128; x++ {
			checkSigned(8, d, x)
		}
	}
	state := uint64(0x1234567812345678)
	for _, n := range []uint{16, 32, 64} {
		mask := ^uint64(0) >> (64 - n)
		for i := range 1000 {
			state = state*6364136223846793005 + 1442695040888963407
			x := state & mask
			d := ((state >> 17) | 1) & mask
			if i%4 == 0 {
				x = mask - uint64(i)
			}
			checkUnsigned(n, d, x)
			sd := int64(d & (mask >> 1))
			if sd == 0 {
				sd = 1
			}
			sx := int64(x<<(64-n)) >> (64 - n)
			checkSigned(n, sd, sx)
		}
	}
}

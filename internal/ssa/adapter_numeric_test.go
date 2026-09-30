package ssa

import (
	"math"
	"math/big"
	"math/rand"
	"testing"
)

// Use unbounded signed arithmetic and modular reduction to avoid repeating the
// native fixed-width overflow formulas in the expectation.
func TestNativeFlagsAgainstUnboundedArithmetic(t *testing.T) {
	random := rand.New(rand.NewSource(51087))
	values := []int64{0, 1, -1, math.MinInt64, math.MaxInt64, math.MinInt32, math.MaxInt32, 1 << 31, 1<<32 - 1}
	for i := 0; i < 128; i++ {
		values = append(values, int64(random.Uint64()))
	}
	for _, width := range []uint{32, 64} {
		modulus := new(big.Int).Lsh(big.NewInt(1), width)
		midpoint := new(big.Int).Rsh(new(big.Int).Set(modulus), 1)
		min := new(big.Int).Neg(midpoint)
		max := new(big.Int).Sub(midpoint, big.NewInt(1))
		for _, a := range values {
			for _, b := range values {
				x, y := a, b
				if width == 32 {
					x, y = int64(int32(a)), int64(int32(b))
				}
				bx, by := big.NewInt(x), big.NewInt(y)
				ux := new(big.Int).Mod(bx, modulus)
				uy := new(big.Int).Mod(by, modulus)
				for _, subtract := range []bool{false, true} {
					exact := new(big.Int)
					var carry bool
					var got flagConstant
					if subtract {
						exact.Sub(bx, by)
						carry = ux.Cmp(uy) >= 0
						if width == 32 {
							got = subFlags32(int32(x), int32(y))
						} else {
							got = subFlags64(x, y)
						}
					} else {
						exact.Add(bx, by)
						carry = new(big.Int).Add(ux, uy).Cmp(modulus) >= 0
						if width == 32 {
							got = addFlags32(int32(x), int32(y))
						} else {
							got = addFlags64(x, y)
						}
					}
					reduced := new(big.Int).Mod(exact, modulus)
					negative := reduced.Cmp(midpoint) >= 0
					zero := reduced.Sign() == 0
					overflow := exact.Cmp(min) < 0 || exact.Cmp(max) > 0
					if got.N() != negative || got.Z() != zero || got.C() != carry || got.V() != overflow {
						t.Fatalf("width=%d x=%d y=%d subtract=%t: %s, want N=%t Z=%t C=%t V=%t", width, x, y, subtract, got, negative, zero, carry, overflow)
					}
					if subtract && (got.eq() != (x == y) || got.lt() != (x < y) || got.le() != (x <= y) || got.gt() != (x > y) || got.ge() != (x >= y) || got.ult() != (ux.Cmp(uy) < 0) || got.ule() != (ux.Cmp(uy) <= 0) || got.ugt() != (ux.Cmp(uy) > 0) || got.uge() != (ux.Cmp(uy) >= 0)) {
						t.Fatalf("comparison width=%d x=%d y=%d flags=%s", width, x, y, got)
					}
				}
			}
		}
	}
}

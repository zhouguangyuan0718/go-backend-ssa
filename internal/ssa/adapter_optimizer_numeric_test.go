package ssa

import (
	"math"
	"math/bits"
	"testing"
)

// Exhaust all 8-bit intervals against arithmetic results, independent of the
// interval formulas used by prove. Require containment for binary arithmetic.
func TestNativeOptimizerIntervals(t *testing.T) {
	for _, tc := range []struct {
		name  string
		bound func(limit, uint) limit
		eval  func(uint8) uint8
	}{
		{"neg", limit.neg, func(x uint8) uint8 { return -x }},
		{"com", limit.com, func(x uint8) uint8 { return ^x }},
		{"ctz", limit.ctz, func(x uint8) uint8 { return uint8(bits.TrailingZeros8(x)) }},
		{"bitlen", limit.bitlen, func(x uint8) uint8 { return uint8(bits.Len8(x)) }},
		{"popcount", limit.popcount, func(x uint8) uint8 { return uint8(bits.OnesCount8(x)) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for lo := 0; lo < 256; lo++ {
				low, high := uint8(255), uint8(0)
				for hi := lo; hi < 256; hi++ {
					x := tc.eval(uint8(hi))
					low = min(low, x)
					high = max(high, x)
					l := tc.bound(limit{math.MinInt64, math.MaxInt64, uint64(lo), uint64(hi)}, 8).intersect(noLimitForBitsize(8))
					if l.umin != uint64(low) || l.umax != uint64(high) {
						t.Fatalf("%d..%d -> %v, actual %d..%d", lo, hi, l, low, high)
					}
				}
			}
		})
	}
	for _, tc := range []struct {
		name  string
		bound func(limit, uint) limit
		eval  func(int8) int8
	}{
		{"signed-neg", limit.neg, func(x int8) int8 { return -x }},
		{"signed-com", limit.com, func(x int8) int8 { return ^x }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for lo := -128; lo <= 127; lo++ {
				low, high := int8(127), int8(-128)
				for hi := lo; hi <= 127; hi++ {
					x := tc.eval(int8(hi))
					low = min(low, x)
					high = max(high, x)
					l := tc.bound(limit{int64(lo), int64(hi), 0, math.MaxUint64}, 8).intersect(noLimitForBitsize(8))
					if l.min != int64(low) || l.max != int64(high) {
						t.Fatalf("%d..%d -> %v, actual %d..%d", lo, hi, l, low, high)
					}
				}
			}
		})
	}
	seed := uint64(123456789)
	next := func() uint64 { seed ^= seed << 13; seed ^= seed >> 7; seed ^= seed << 17; return seed }
	for _, width := range []uint{8, 16, 32, 64} {
		mask := uint64(math.MaxUint64) >> (64 - width)
		signed := func(x uint64) int64 { return int64(x<<(64-width)) >> (64 - width) }
		for sample := 0; sample < 1000; sample++ {
			a, b := next()&mask, next()&mask
			la := limit{signed(a), signed(a), a, a}
			lb := limit{signed(b), signed(b), b, b}
			for _, tc := range []struct {
				l    limit
				want uint64
			}{{la.add(lb, width), (a + b) & mask}, {la.sub(lb, width), (a - b) & mask}, {la.mul(lb, width), (a * b) & mask}} {
				if tc.l.min > signed(tc.want) || tc.l.max < signed(tc.want) || tc.l.umin > tc.want || tc.l.umax < tc.want {
					t.Fatalf("width=%d a=%x b=%x bound=%v excludes %x", width, a, b, tc.l, tc.want)
				}
			}
		}
	}
}

func TestNativeOptimizerKnownBitsValues(t *testing.T) {
	// Enumeration must cover every assignment of the unknown bits exactly once.
	for known := 0; known < 256; known++ {
		for value := 0; value < 256; value++ {
			if value & ^known != 0 {
				continue
			}
			seen := map[int64]bool{}
			for x := range allPossibleValues(int64(value), int64(known)|^int64(255)) {
				if x < 0 || x > 255 || int(x)&known != value || seen[x] {
					t.Fatalf("bad enumeration %x/%x -> %x", value, known, x)
				}
				seen[x] = true
			}
			if len(seen) != 1<<bits.OnesCount8(^uint8(known)) {
				t.Fatal("missing bit assignments")
			}
		}
	}
}

package liveness

import (
	"math/rand"
	"testing"
)

// Compare the native interval algorithms with an independent dense set model.
func TestNativeIntervalSets(t *testing.T) {
	r := rand.New(rand.NewSource(481516))
	c := new(Context)
	for trial := 0; trial < 1000; trial++ {
		var a, b [64]bool
		for i := range a {
			a[i] = r.Intn(3) == 0
			b[i] = r.Intn(3) == 0
		}
		build := func(bits [64]bool) Intervals {
			builder := &c.bindIntervalBuilders(make([]IntervalsBuilder, 1))[0]
			for i := len(bits) - 1; i >= 0; i-- {
				var err error
				if bits[i] {
					err = builder.Live(i)
				} else {
					err = builder.Kill(i)
				}
				if err != nil {
					t.Fatal(err)
				}
			}
			got, err := builder.Finish()
			if err != nil {
				t.Fatal(err)
			}
			return got
		}
		aa, bb := build(a), build(b)
		overlap := false
		for i := range a {
			overlap = overlap || a[i] && b[i]
		}
		if aa.Overlaps(bb) != overlap || bb.Overlaps(aa) != overlap {
			t.Fatal("overlap differs from point sets")
		}
		for _, result := range []Intervals{aa.Merge(bb), bb.Merge(aa)} {
			if err := check(result); err != nil {
				t.Fatal(err)
			}
			var got [64]bool
			for _, span := range result {
				for i := span.st; i < span.en; i++ {
					if got[i] {
						t.Fatal("overlapping merged intervals")
					}
					got[i] = true
				}
			}
			for i := range a {
				if got[i] != (a[i] || b[i]) {
					t.Fatalf("union differs at %d", i)
				}
			}
		}
	}
}
func TestNativeIntervalFailures(t *testing.T) {
	c := new(Context)
	b := &c.bindIntervalBuilders(make([]IntervalsBuilder, 1))[0]
	if b.Live(-1) == nil || b.Kill(-1) == nil {
		t.Fatal("negative position accepted")
	}
	if err := b.Live(4); err != nil {
		t.Fatal(err)
	}
	if b.Live(4) == nil || b.Kill(5) == nil {
		t.Fatal("nondecreasing position accepted")
	}
	if err := b.Kill(1); err != nil {
		t.Fatal(err)
	}
	spans, err := b.Finish()
	if err != nil || len(spans) != 1 || spans[0] != (Interval{st: 2, en: 5}) {
		t.Fatalf("%v %v", spans, err)
	}
	for _, bad := range []Intervals{{{st: 2, en: 2}}, {{st: 3, en: 2}}, {{st: 1, en: 4}, {st: 3, en: 5}}} {
		if check(bad) == nil {
			t.Fatal("invalid intervals accepted", bad)
		}
	}
}

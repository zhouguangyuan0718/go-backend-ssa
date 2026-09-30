package ssa

import (
	"runtime"
	"sync"
	"testing"
)

func TestNativeDerivedAllocatorIsolation(t *testing.T) {
	var retained Cache
	pointers := retained.allocBlockSlice(33)
	pointers[0] = &Block{ID: 991, Values: []*Value{{ID: 777}}}
	numbers := retained.allocInt8Slice(257)
	for i := range numbers {
		numbers[i] = int8(i)
	}
	var wg sync.WaitGroup
	for worker := 0; worker < 8; worker++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			var c Cache
			for round := 0; round < 32; round++ {
				n := 1 + (round*19+worker)%257
				blocks := c.allocBlockSlice(n)
				ints := c.allocIntSlice(n)
				known := c.allocKnownBitsEntriesSlice(n)
				flags := c.allocBoolSlice(n)
				for i := 0; i < n; i++ {
					if blocks[i] != nil || ints[i] != 0 || known[i] != (knownBitsEntry{}) || flags[i] {
						t.Error("derived slice retained data")
					}
					blocks[i] = &Block{ID: ID(i + 1)}
					ints[i] = i + worker
					known[i] = knownBitsEntry{known: -1, value: int64(i)}
					flags[i] = true
				}
				c.freeBlockSlice(blocks)
				c.freeIntSlice(ints)
				c.freeKnownBitsEntriesSlice(known)
				c.freeBoolSlice(flags)
			}
		}(worker)
	}
	wg.Wait()
	runtime.GC()
	if pointers[0].ID != 991 || pointers[0].Values[0].ID != 777 {
		t.Fatal("reinterpreted pointer slice lost its GC roots")
	}
	for i, n := range numbers {
		if n != int8(i) {
			t.Fatal("independent allocator changed retained numeric storage")
		}
	}
	retained.freeBlockSlice(pointers)
	retained.freeInt8Slice(numbers)
}

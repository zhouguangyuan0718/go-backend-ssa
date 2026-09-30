package ssa

import (
	"sync"
	"testing"

	"go-backend-ssa/internal/cmd/src"
)

func TestNativeCachePoolIsolation(t *testing.T) {
	var retained Cache
	marker := &Value{ID: 987}
	held := retained.allocValueSlice(32)
	held[0] = marker
	var wg sync.WaitGroup
	for worker := 0; worker < 8; worker++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			var c Cache // No context constructor or Reset is required for pool initialization.
			for round := 0; round < 40; round++ {
				n := 1 + (round*19+worker)%257
				values := c.allocValueSlice(n)
				limits := c.allocLimitSlice(n)
				set := c.allocSparseSet(n)
				m := c.allocSparseMap(n)
				pos := c.allocSparseMapPos(n)
				for _, v := range values {
					if v != nil {
						t.Error("pool retained a value reference")
					}
				}
				for _, l := range limits {
					if l != (limit{}) {
						t.Error("pool retained numeric scratch state")
					}
				}
				if set.size() != 0 || m.size() != 0 || pos.size() != 0 {
					t.Error("pool retained sparse contents")
				}
				values[n-1] = &Value{ID: ID(worker + 1)}
				limits[n-1] = limit{min: -1, max: int64(n)}
				set.add(ID(n - 1))
				m.set(ID(n-1), int32(worker))
				pos.set(ID(n-1), int32(worker), src.XPos{})
				c.freeValueSlice(values)
				c.freeLimitSlice(limits)
				c.freeSparseSet(set)
				c.freeSparseMap(m)
				c.freeSparseMapPos(pos)
			}
			// Use the actual relocated pool fields: an empty, independent Cache must
			// never receive another instance's entries. sync.Pool may drop its own entry.
			var other Cache
			if other.poolFreeValueSlice[0].Get() != nil || other.poolFreeLimitSlice[0].Get() != nil || other.poolFreeSparseSet[0].Get() != nil || other.poolFreeSparseMap[0].Get() != nil || other.poolFreeSparseMapPos[0].Get() != nil {
				t.Error("independent cache shared pools")
			}
		}(worker)
	}
	wg.Wait()
	if held[0] != marker || held[0].ID != 987 {
		t.Fatal("live scratch slice changed during independent allocations")
	}
	retained.freeValueSlice(held)
}

func TestNativeSparseContainerModels(t *testing.T) {
	// Exercise the generic map against a language map through deterministic mixed
	// mutations. Check the complete key space after every operation, including the
	// dense removal path and clears that leave the sparse index storage intact.
	m := newGenericSparseMap[int, string](31)
	want := map[int]string{}
	for step := 0; step < 400; step++ {
		key := (step*13 + step/7) % 31
		switch step % 9 {
		case 0, 1:
			m.remove(key)
			delete(want, key)
		case 8:
			m.clear()
			clear(want)
		default:
			value := string(rune('a' + step%26))
			m.set(key, value)
			want[key] = value
		}
		if m.size() != len(want) {
			t.Fatal("generic sparse map size")
		}
		for k := 0; k < 31; k++ {
			got, ok := m.get(k)
			value, present := want[k]
			if got != value || ok != present || m.contains(k) != present {
				t.Fatalf("step %d key %d: got %q/%t, want %q/%t", step, k, got, ok, value, present)
			}
		}
	}
}

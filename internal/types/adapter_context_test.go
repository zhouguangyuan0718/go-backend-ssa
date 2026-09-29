package types

import (
	"fmt"
	"sync"
	"testing"
)

func TestContextOwnershipAndInputLifetime(t *testing.T) {
	a, b := NewUniverse(8, 8, 1<<50), NewUniverse(8, 8, 1<<50)
	input := []byte("alpha")
	old := a.InternString(input)
	input[0] = 'z'
	if old != "alpha" {
		t.Fatal("returned string aliases caller buffer")
	}
	if len(a.strings) != 1 || len(b.strings) != 0 {
		t.Fatal("shared state")
	}
	for i := 0; i < 100; i++ {
		NewUniverse(8, 8, 1<<50).InternString([]byte("other"))
	}
	if old != "alpha" || a.InternString([]byte("alpha")) != old {
		t.Fatal("old result invalidated")
	}
	b.InternString([]byte("beta"))
	if len(a.strings) != 1 {
		t.Fatal("other context mutated first")
	}
}

func TestConcurrentIndependentContexts(t *testing.T) {
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			c := NewUniverse(8, 8, 1<<50)
			for j := 0; j < 100; j++ {
				want := fmt.Sprintf("%d/%d", i, j)
				if got := c.InternString([]byte(want)); got != want {
					t.Errorf("got %q, want %q", got, want)
				}
			}
			if len(c.strings) != 100 {
				t.Errorf("context contains another call's strings")
			}
		}(i)
	}
	wg.Wait()
}

package types_test

import (
	"go-backend-ssa/types"
	"strings"
	"sync"
	"testing"
)

func TestCompositeConstructorsAndPointerCache(t *testing.T) {
	a, b := universe(t, "amd64"), universe(t, "386")
	x, _ := a.Scalar(types.Int)
	y, _ := b.Scalar(types.Int)
	x.SetHasShape(true)
	x.SetNoalg(true)
	x.SetNotInHeap(true)
	ptr := a.NewPtr(x)
	slice := a.NewSlice(x)
	array := a.NewArray(x, 7)
	channel := a.NewChan(x, types.RecvOnly)
	mapping := a.NewMap(x, x)
	for _, typ := range []*types.Type{ptr, slice, array, channel, mapping} {
		if typ.Elem() != x || typ.Universe() != a || !typ.HasShape() {
			t.Fatal("composite owner/element/flags lost")
		}
	}
	if ptr != a.NewPtr(x) || x.PtrTo() != ptr || slice != a.NewSlice(x) || !ptr.IsPtr() || !ptr.Noalg() || !slice.IsSlice() || !array.IsArray() || array.NumElem() != 7 || !array.NotInHeap() {
		t.Fatal("upstream cache or flags changed")
	}
	if !channel.IsChan() || channel.ChanDir() != types.RecvOnly || !channel.ChanDir().CanRecv() || channel.ChanDir().CanSend() || !mapping.IsMap() || mapping.Key() != x || mapping.MapType().Elem != x {
		t.Fatal("composite accessors changed")
	}
	if b.NewPtr(y) == ptr || b.NewSlice(y) == slice || y.HasShape() {
		t.Fatal("cross-instance cache")
	}
	a.NewPtrCacheEnabled = false
	uncached, _ := a.Scalar(types.Bool)
	if a.NewPtr(uncached) == a.NewPtr(uncached) || b.NewPtr(y) != b.NewPtr(y) || a.NewPtr(x) != ptr {
		t.Fatal("cache switch must only disable new cache writes, per upstream")
	}
	if _, err := a.Scalar(types.Array); err == nil {
		t.Fatal("non-scalar accepted")
	}
	for name, f := range map[string]func(){
		"foreign pointer": func() { a.NewPtr(y) }, "foreign slice": func() { a.NewSlice(y) }, "foreign array": func() { a.NewArray(y, 3) }, "foreign map": func() { a.NewMap(x, y) }, "foreign channel": func() { a.NewChan(y, types.SendRecv) }, "nil element": func() { a.NewSlice(nil) },
	} {
		t.Run(name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Error("invalid graph accepted")
				}
			}()
			f()
		})
	}
	func() {
		defer func() {
			r := recover()
			err, ok := r.(error)
			if !ok || !strings.Contains(err.Error(), "invalid bound -1") {
				t.Errorf("fatal did not become a recoverable error: %v", r)
			}
		}()
		a.NewArray(x, -1)
	}()
	// The invariant failure neither exits the host nor poisons another call.
	if a.NewArray(x, 2).NumElem() != 2 || b.NewPtr(y).Elem() != y {
		t.Fatal("failure leaked state")
	}
}

func TestConcurrentCompositeUniverses(t *testing.T) {
	var wg sync.WaitGroup
	for i := 0; i < 24; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			u := universe(t, []string{"amd64", "386", "arm64"}[i%3])
			x, _ := u.Scalar(types.Int)
			u.NewPtrCacheEnabled = i%2 == 0
			for j := 0; j < 50; j++ {
				p := u.NewPtr(x)
				s := u.NewSlice(p)
				a := u.NewArray(s, int64(j))
				if a.Elem().Elem().Elem() != x || a.Universe() != u {
					t.Error("foreign graph")
					return
				}
			}
		}(i)
	}
	wg.Wait()
}

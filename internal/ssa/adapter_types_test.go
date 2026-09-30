package ssa

import (
	"fmt"
	"reflect"
	"sync"
	"testing"

	"go-backend-ssa/internal/types"
)

// Enumerate the actual native fields so upstream additions cannot silently
// escape the identity and retention tests.
func checkTypeCollection(t *testing.T, got *Types, u *types.Universe) {
	t.Helper()
	v := reflect.ValueOf(got).Elem()
	for i := 0; i < v.NumField(); i++ {
		if !v.Field(i).CanInterface() {
			continue
		}
		typ, ok := v.Field(i).Interface().(*types.Type)
		if !ok || typ == nil || typ.Universe() != u {
			t.Fatalf("%s: missing or foreign native type", v.Type().Field(i).Name)
		}
	}
	if got.Int != u.Types[types.TINT] || got.BytePtr != u.NewPtr(u.Types[types.TUINT8]) || got.BytePtrPtr != u.NewPtr(got.BytePtr) || got.Vec512 != u.TypeVec512 || got.Mask != u.TypeMask {
		t.Fatal("lost canonical native type identities")
	}
	if got.Int.Size() != int64(u.PointerSize()) || got.BytePtr.Size() != int64(u.PointerSize()) {
		t.Fatal("wrong target layout")
	}
}

func TestNativeTypeCollection(t *testing.T) {
	for _, size := range []int{4, 8} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			u := universe(size)
			a, b := NewTypes(u), NewTypes(u)
			checkTypeCollection(t, a, u)
			if a == b || !reflect.DeepEqual(a, b) {
				t.Fatal("collections must be distinct with canonical shared types")
			}
			saved := *a
			// Config contains Types by value. Copying must preserve its owner, and the
			// native reset must recover the original universe even after field edits.
			a.Int = nil
			a.BytePtr = nil
			a.Vec512 = nil
			a.SetTypPtrs()
			if *a != saved {
				t.Fatal("native SetTypPtrs did not restore canonical collection")
			}
			copied := *a
			copied.SetTypPtrs()
			if copied != saved {
				t.Fatal("copy lost type ownership")
			}
		})
	}
}

func TestTypeCollectionIsolation(t *testing.T) {
	oldUniverse := universe(4)
	old := NewTypes(oldUniverse)
	saved := *old
	var wg sync.WaitGroup
	for j := 0; j < 8; j++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			u := universe(8)
			fresh := NewTypes(u)
			checkTypeCollection(t, fresh, u)
			fresh.Int = nil
			fresh.SetTypPtrs()
			if fresh.Int == old.Int || fresh.BytePtr == old.BytePtr || fresh.Vec512 == old.Vec512 {
				t.Error("independent universes share types")
			}
		}()
	}
	wg.Wait()
	old.SetTypPtrs()
	checkTypeCollection(t, old, oldUniverse)
	if *old != saved {
		t.Fatal("retained type collection changed")
	}
}

func TestTypeCollectionRejectsUnboundInputs(t *testing.T) {
	mustPanic := func(name string, f func()) {
		t.Helper()
		t.Run(name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Fatal("accepted invalid type universe")
				}
			}()
			f()
		})
	}
	mustPanic("nil", func() { NewTypes(nil) })
	mustPanic("uninitialized", func() { NewTypes(types.NewUniverse(8, 8, 1<<50)) })
	mustPanic("zero-collection", func() { new(Types).SetTypPtrs() })
	u := universe(8)
	foreign := universe(8)
	collection := NewTypes(u)
	saved := *collection
	original := u.Types[types.TBOOL]
	u.Types[types.TBOOL] = foreign.Types[types.TBOOL]
	mustPanic("foreign-builtin", func() { collection.SetTypPtrs() })
	if *collection != saved {
		t.Fatal("partial mutation before rejecting foreign builtin")
	}
	u.Types[types.TBOOL] = original
	originalVector := u.TypeVec512
	u.TypeVec512 = foreign.TypeVec512
	mustPanic("foreign-vector", func() { collection.SetTypPtrs() })
	if *collection != saved {
		t.Fatal("partial mutation before rejecting foreign vector")
	}
	u.TypeVec512 = originalVector
	collection.SetTypPtrs()
}

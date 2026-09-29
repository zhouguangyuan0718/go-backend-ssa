package obj

import (
	"go-backend-ssa/internal/cmd/src"
	"sync"
	"sync/atomic"
	"testing"
)

func TestNativeLookupOnceAndABI(t *testing.T) {
	var link Link
	var calls atomic.Int32
	symbols := make([]*LSym, 32)
	var wg sync.WaitGroup
	for i := range symbols {
		wg.Go(func() {
			symbols[i] = link.LookupABIInit("p.F", ABIInternal, func(s *LSym) { calls.Add(1); s.Pkg = "p" })
		})
	}
	wg.Wait()
	if calls.Load() != 1 {
		t.Fatal("native initializer ran", calls.Load())
	}
	for _, s := range symbols {
		if s != symbols[0] || s.Pkg != "p" || s.ABI() != ABIInternal {
			t.Fatal("native interned identity")
		}
	}
	data := link.Lookup("p.F")
	if data == symbols[0] || data != link.LookupABI("p.F", ABI0) {
		t.Fatal("ABI maps mixed")
	}
	link.LookupInit("p.F", func(s *LSym) { s.Pkg = "data" })
	link.LookupABIInit("p.F", ABI0, func(*LSym) { t.Fatal("initializer reran") })
	if data.Pkg != "data" {
		t.Fatal("late one-time initialization")
	}
	data.Set(AttrContentAddressable, true)
	data.SetABI(ABIInternal)
	if data.ABI() != ABIInternal || data.Attribute&AttrContentAddressable == 0 {
		t.Fatal("ABI overwrote attributes")
	}
}

func TestNativeInlinePositions(t *testing.T) {
	var link Link
	outer := src.NewFileBase("outer.go", "/src/outer.go")
	inner := src.NewFileBase("inner.go", "/src/inner.go")
	call := link.PosTable.XPos(src.MakePos(outer, 10, 2))
	f := link.Lookup("p.F")
	f.Pkg = "p"
	i := link.InlTree.Add(-1, call, f, "F")
	middle := link.PosTable.XPos(src.MakePos(src.NewInliningBase(inner, i), 20, 3))
	j := link.InlTree.Add(i, middle, link.Lookup("p.G"), "G")
	leaf := link.PosTable.XPos(src.MakePos(src.NewInliningBase(inner, j), 30, 4))
	var got []uint
	link.AllPos(leaf, func(p src.Pos) { got = append(got, p.Line()) })
	if len(got) != 3 || got[0] != 10 || got[1] != 20 || got[2] != 30 || link.OutermostPos(leaf).Line() != 10 || link.InnermostPos(leaf).Line() != 30 {
		t.Fatal(got)
	}
	if link.InlTree.Parent(j) != i || link.InlTree.InlinedFunction(i) != f || link.InlTree.InlinedFuncPkg(i) != "p" || link.InlTree.InlinedFuncName(i) != "F" || link.InlTree.CallPos(i) != call {
		t.Fatal("inline tree identity")
	}
}

package ssa

import (
	"sync"
	"testing"

	"go-backend-ssa/internal/cmd/src"
)

// These fixtures exercise the native Config.NewFunc and graph methods. A literal
// Config intentionally supplies only their inputs; it does not substitute for
// the architecture-dependent NewConfig factory, which is not migrated yet.
func nativeGraphFixture(size int) *Func {
	u := universe(size)
	cfg := &Config{PtrSize: int64(size), RegSize: int64(size), Types: *NewTypes(u)}
	cache := new(Cache)
	cache.Reset()
	f := cfg.NewFunc(nil, cache)
	f.Entry = f.NewBlock(BlockPlain)
	return f
}

func checkNativeGraph(t *testing.T, f *Func) {
	t.Helper()
	uses := map[*Value]int32{}
	seen := map[ID]bool{}
	for _, b := range f.Blocks {
		if b.Func != f {
			t.Fatal("block lost function")
		}
		for i, e := range b.Succs {
			back := e.b.Preds[e.i]
			if back.b != b || back.i != i {
				t.Fatal("successor lost reverse edge")
			}
		}
		for i, e := range b.Preds {
			back := e.b.Succs[e.i]
			if back.b != b || back.i != i {
				t.Fatal("predecessor lost reverse edge")
			}
		}
		for _, v := range b.ControlValues() {
			uses[v]++
		}
		for _, v := range b.Values {
			if v.Block != b || v.ID == 0 || seen[v.ID] {
				t.Fatal("value lost identity or block")
			}
			seen[v.ID] = true
			if v.Type.Universe() != f.Config.Types.Int.Universe() {
				t.Fatal("foreign graph type")
			}
			for _, arg := range v.Args {
				uses[arg]++
			}
		}
	}
	for _, b := range f.Blocks {
		for _, v := range b.Values {
			if v.Uses != uses[v] {
				t.Fatalf("%s: uses=%d, graph=%d", v, v.Uses, uses[v])
			}
		}
	}
}

func fillNativeGraph(f *Func) {
	typ := f.Config.Types.Int
	op := OpConst64
	if f.Config.PtrSize == 4 {
		op = OpConst32
	}
	for i := 0; i < 2200; i++ {
		v := f.Entry.NewValue0I(src.XPos{}, op, typ, int64(i))
		if i > 0 {
			v.AddArgs(f.Entry.Values[i-1], f.Entry.Values[0])
		}
	}
	for i := 0; i < 220; i++ {
		b := f.NewBlock(BlockIf)
		f.Entry.AddEdgeTo(b)
		b.AddEdgeTo(b)
		b.AddEdgeTo(f.Entry)
		b.SetControl(f.Entry.Values[i])
		b.AddControl(f.Entry.Values[i])
		b.ReplaceControl(0, f.Entry.Values[i+1])
		b.swapSuccessorsByIdx(0, 1)
	}
}

func TestNativeGraphConstructionIsolation(t *testing.T) {
	retained := nativeGraphFixture(4)
	fillNativeGraph(retained)
	checkNativeGraph(t, retained)
	first := retained.Entry.Values[0]
	retained.NamedValues[LocalSlot{Type: first.Type}] = []*Value{first}
	var wg sync.WaitGroup
	for worker := 0; worker < 8; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			f := nativeGraphFixture(8)
			fillNativeGraph(f)
			checkNativeGraph(t, f)
			if f.Entry == retained.Entry || f.Cache == retained.Cache || f.OwnAux == retained.OwnAux || f.Config.Types.Int == first.Type {
				t.Error("independent graph shared storage")
			}
			// Resetting this graph's cache must not change any separately owned graph.
			f.Cache.Reset()
			checkNativeGraph(t, retained)
		}()
	}
	wg.Wait()
	checkNativeGraph(t, retained)
	if first.AuxInt != 0 || first.Type.Size() != 4 || retained.Entry.Values[2199].AuxInt != 2199 || len(retained.NamedValues) != 1 {
		t.Fatal("retained graph changed")
	}
}

func TestNativeCacheLifetime(t *testing.T) {
	f := nativeGraphFixture(8)
	fillNativeGraph(f)
	cache := f.Cache
	oldBlock, oldValue := f.Entry, f.Entry.Values[0]
	if oldBlock != &cache.blocks[1] || oldValue != &cache.values[1] {
		t.Fatal("native cache allocation not used")
	}
	// Native Reset invalidates the previous graph. Preserve that contract rather
	// than hiding a cache reuse bug with copies or silently changing its algorithm.
	cache.Reset()
	if oldBlock.ID != 0 || oldValue.ID != 0 {
		t.Fatal("native cache reset did not invalidate owned graph")
	}
	next := f.Config.NewFunc(nil, cache)
	next.Entry = next.NewBlock(BlockPlain)
	value := next.Entry.NewValue0(src.XPos{}, OpUnknown, next.Config.Types.Int)
	if next.Entry != oldBlock || value != oldValue || value.ID != 1 || next.Entry.ID != 1 {
		t.Fatal("cache was not reused with fresh identities")
	}
	checkNativeGraph(t, next)
}

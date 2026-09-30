package ssa

import (
	"strings"
	"sync"
	"testing"

	"go-backend-ssa/internal/cmd/src"
	"go-backend-ssa/internal/ir"
)

func TestNativeSSACheckCleanupPipeline(t *testing.T) {
	run := func(size int) *Func {
		f, _, _, _, _, trace := deadcodeFixture(size)
		f.pass.debug = 5
		checkFunc(f)
		if f.loopnest() != f.cachedLoopnest {
			t.Error("loop analysis not cached")
		}
		numberLines(f)
		deadcode(f)
		f.invalidateCFG()
		checkFunc(f)
		if !strings.Contains(trace.String(), "loop finding") {
			t.Error("loop trace lost compilation binding")
		}
		f.scheduled = true
		checkFunc(f)
		return f
	}
	retained := run(4)
	before := retained.String()
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			f := run(8)
			if f.Config.Types.Int.Size() != 8 || retained.Config.Types.Int.Size() != 4 {
				t.Error("target state leaked")
			}
		})
	}
	wg.Wait()
	if retained.String() != before {
		t.Fatal("retained checked graph changed")
	}
}

func TestNativeSSACheckRejectsCorruption(t *testing.T) {
	for _, test := range []struct {
		name, match string
		breakGraph  func(*Func)
	}{
		{"uses", "uses", func(f *Func) { f.Entry.Values[0].Uses++ }},
		{"block", "appears twice", func(f *Func) { f.Blocks = append(f.Blocks, f.Entry) }},
		{"value", "appears twice", func(f *Func) { f.Entry.Values = append(f.Entry.Values, f.Entry.Values[0]) }},
		{"control", "nil control", func(f *Func) { f.Blocks[len(f.Blocks)-1].ResetControls() }},
	} {
		t.Run(test.name, func(t *testing.T) {
			f, _, _, _, _, _ := deadcodeFixture(8)
			numberLines(f)
			deadcode(f)
			f.invalidateCFG()
			checkFunc(f)
			test.breakGraph(f)
			defer func() {
				r := recover()
				if r == nil || !strings.Contains(r.(string), test.match) {
					t.Fatalf("unexpected rejection %v", r)
				}
			}()
			checkFunc(f)
		})
	}
}

func TestNativeMergeCandidatePolicyOwnership(t *testing.T) {
	makeName := func(size int, enabled int) *ir.Name {
		u, _ := auxFixture(size)
		// A separate graph explicitly owns this name; policy must be derived from
		// the name, even when another graph uses the same type universe.
		graph := ir.NewContext(u, u.LinkContext().Ctxt)
		graph.Debug.MergeLocals = enabled
		// Use a pointer-sized scalar so the native threshold is exactly 3*PtrSize.
		n := graph.NewNameAt(src.XPos{}, u.LocalPkg.Lookup("local"), u.NewArray(NewTypes(u).Uintptr, 4))
		n.Class = ir.PAUTO
		if ir.ContextOf(n) != graph {
			t.Fatal("IR owner changed")
		}
		return n
	}
	retained := makeName(4, 1)
	if !IsMergeCandidate(retained) {
		t.Fatal("large local rejected")
	}
	other := makeName(8, 0)
	if IsMergeCandidate(other) || !IsMergeCandidate(retained) {
		t.Fatal("merge policy leaked")
	}
	graph := ir.ContextOf(retained)
	graph.Base.Flag.N = 1
	if IsMergeCandidate(retained) {
		t.Fatal("optimization-disabled local accepted")
	}
	graph.Base.Flag.N = 0
	graph.Debug.MergeLocals = 0
	if IsMergeCandidate(retained) {
		t.Fatal("disabled merge accepted")
	}
	graph.Debug.MergeLocals = 1
	for name, change := range map[string]func(){"unowned": func() { IsMergeCandidate(new(ir.Name)) }, "foreign-type": func() { retained.SetType(other.Type()); IsMergeCandidate(retained) }, "nil": func() { ir.ContextOf(nil) }} {
		t.Run(name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Fatal("unowned input accepted")
				}
			}()
			change()
		})
	}
}

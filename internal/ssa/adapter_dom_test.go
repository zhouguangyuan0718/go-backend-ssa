package ssa

import (
	"sync"
	"testing"
)

func graphForDominators(n int, edges [][2]int) *Func {
	f := new(Config).NewFunc(nil, new(Cache))
	for i := 0; i < n; i++ {
		f.NewBlock(BlockPlain)
	}
	f.Entry = f.Blocks[0]
	for _, e := range edges {
		f.Blocks[e[0]].AddEdgeTo(f.Blocks[e[1]])
	}
	return f
}

// Removing a vertex disconnects exactly the reachable vertices it dominates.
// This deliberately does not use SSA's DFS numbering, scratch arrays, or LT
// recurrence, so a shared mistake in those mechanisms cannot satisfy the model.
func reachableWithout(f *Func, excluded *Block) map[*Block]bool {
	seen := map[*Block]bool{}
	if f.Entry == excluded {
		return seen
	}
	queue := []*Block{f.Entry}
	seen[f.Entry] = true
	for len(queue) > 0 {
		b := queue[0]
		queue = queue[1:]
		for _, e := range b.Succs {
			if e.b != excluded && !seen[e.b] {
				seen[e.b] = true
				queue = append(queue, e.b)
			}
		}
	}
	return seen
}

func checkDominators(t *testing.T, f *Func) {
	t.Helper()
	live := reachableWithout(f, nil)
	dom := map[*Block]map[*Block]bool{}
	for _, a := range f.Blocks {
		without := reachableWithout(f, a)
		dom[a] = map[*Block]bool{}
		for _, b := range f.Blocks {
			dom[a][b] = live[a] && live[b] && !without[b]
		}
	}
	po := f.Postorder()
	if len(po) != len(live) {
		t.Fatal("postorder includes unreachable blocks or omits live blocks")
	}
	seen := map[*Block]bool{}
	for _, b := range po {
		if !live[b] || seen[b] {
			t.Fatal("postorder identity mismatch")
		}
		seen[b] = true
	}
	if po[len(po)-1] != f.Entry {
		t.Fatal("entry is not last in postorder")
	}
	idom, tree := f.Idom(), f.Sdom()
	for _, b := range f.Blocks {
		var want *Block
		if live[b] && b != f.Entry {
			for _, d := range f.Blocks {
				if d == b || !dom[d][b] {
					continue
				}
				closest := true
				for _, other := range f.Blocks {
					if other != b && other != d && dom[other][b] && dom[d][other] {
						closest = false
						break
					}
				}
				if closest {
					if want != nil {
						t.Fatal("model has multiple immediate dominators")
					}
					want = d
				}
			}
			if want == nil {
				t.Fatal("model has no immediate dominator")
			}
		}
		if idom[b.ID] != want || tree.Parent(b) != want {
			t.Fatalf("block %d: idom=%v tree=%v want=%v", b.ID, idom[b.ID], tree.Parent(b), want)
		}
		if !live[b] {
			continue
		}
		for _, a := range f.Blocks {
			if !live[a] {
				continue
			}
			if tree.IsAncestorEq(a, b) != dom[a][b] || tree.isAncestor(a, b) != (a != b && dom[a][b]) {
				t.Fatalf("wrong sparse domination: %v -> %v", a, b)
			}
		}
	}
	// Repeated queries should retain the native cached results until a CFG edit.
	if &po[0] != &f.Postorder()[0] || &idom[0] != &f.Idom()[0] || &tree[0] != &f.Sdom()[0] {
		t.Fatal("native graph analysis caches were not reused")
	}
}

func TestNativeDominatorsAgainstReachability(t *testing.T) {
	fixtures := [][][2]int{
		{}, {{0, 1}, {1, 2}, {2, 3}}, {{0, 1}, {0, 2}, {1, 3}, {2, 3}},
		{{0, 1}, {1, 1}, {1, 2}, {2, 1}, {2, 3}},
		{{0, 1}, {0, 2}, {1, 2}, {2, 1}, {1, 3}, {2, 3}},
		{{0, 1}, {0, 1}, {1, 2}, {3, 2}, {3, 4}, {4, 3}},
		{{0, 1}, {1, 2}, {2, 0}, {2, 3}},
	}
	for i, edges := range fixtures {
		f := graphForDominators(6, edges)
		checkDominators(t, f)
		// Adding an edge can make an unreachable component reachable and remove
		// an existing dominator. All three native caches must be invalidated.
		f.Blocks[0].AddEdgeTo(f.Blocks[3])
		if f.cachedPostorder != nil || f.cachedIdom != nil || f.cachedSdom != nil {
			t.Fatalf("fixture %d retained stale analyses", i)
		}
		checkDominators(t, f)
		e := f.Blocks[0].Succs[len(f.Blocks[0].Succs)-1]
		e.b.removePred(e.i)
		f.Blocks[0].removeSucc(len(f.Blocks[0].Succs) - 1)
		checkDominators(t, f)
	}
	state := uint32(1234567)
	for sample := 0; sample < 80; sample++ {
		n := 2 + sample%19
		var edges [][2]int
		for i := 0; i < n*n/3; i++ {
			state ^= state << 13
			state ^= state >> 17
			state ^= state << 5
			a := int(state % uint32(n))
			state = state*1664525 + 1013904223
			b := int(state % uint32(n))
			edges = append(edges, [2]int{a, b})
		}
		f := graphForDominators(n, edges)
		f.Entry = f.Blocks[sample%n]
		checkDominators(t, f)
	}
}

func TestNativeDominatorGraphIsolation(t *testing.T) {
	old := graphForDominators(6, [][2]int{{0, 1}, {0, 2}, {1, 3}, {2, 3}, {3, 4}, {4, 3}})
	checkDominators(t, old)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			f := graphForDominators(270, nil)
			for j := 1; j < len(f.Blocks); j++ {
				f.Blocks[j-1].AddEdgeTo(f.Blocks[j])
			}
			for j := 0; j < 3; j++ {
				idom := f.Idom()
				for k := 1; k < len(f.Blocks); k++ {
					if idom[f.Blocks[k].ID] != f.Blocks[k-1] {
						t.Error("long chain dominator")
					}
				}
				f.invalidateCFG()
			}
		}()
	}
	wg.Wait()
	checkDominators(t, old)
}

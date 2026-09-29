package dwarfgen

import (
	"go-backend-ssa/internal/cmd/obj"
	"go-backend-ssa/internal/cmd/src"
	"go-backend-ssa/internal/ir"
	"go-backend-ssa/internal/typecheck"
	"go-backend-ssa/internal/types"
	"reflect"
	"sync"
	"testing"
)

func markerGraph() *ir.Context {
	link := &obj.Link{}
	u := types.NewUniverseWithPositions(8, 8, 1<<50, &link.PosTable)
	u.LocalPkg = u.NewPkg("example/p", "p")
	g := ir.NewContext(u, link)
	typecheck.NewContext(g).InitUniverse()
	return g
}
func TestNativeScopeMarkerConcurrentAndRetained(t *testing.T) {
	var wg sync.WaitGroup
	for range 12 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			g := markerGraph()
			b := src.NewFileBase("f.go", "/src/f.go")
			pos := func(line uint) src.XPos { return g.Base.Ctxt.PosTable.XPos(src.MakePos(b, line, 1)) }
			m := NewScopeMarker(g.Base)
			fn := g.NewFunc(pos(1), pos(1), g.Types.LocalPkg.Lookup("F"), g.Types.NewSignature(nil, nil, nil))
			m.Push(pos(1))
			m.Push(pos(2))
			m.Unpush()
			m.Push(pos(3))
			m.Pop(pos(3))
			m.Pop(pos(4))
			m.WriteTo(fn)
			want := []ir.Mark{{Pos: pos(1), Scope: 1}, {Pos: pos(3), Scope: 1}, {Pos: pos(4), Scope: 0}}
			if !reflect.DeepEqual(fn.Parents, []ir.ScopeID{0, 1}) || !reflect.DeepEqual(fn.Marks, want) {
				t.Error("native scope compaction", fn.Parents, fn.Marks)
			}
			other := g.NewFunc(pos(5), pos(5), g.Types.LocalPkg.Lookup("G"), fn.Type())
			m.Push(pos(5))
			m.Pop(pos(6))
			m.WriteTo(other)
			if !reflect.DeepEqual(fn.Marks, want) || len(fn.Parents) != 2 || len(other.Parents) != 1 {
				t.Error("reused marker overwrote old function")
			}
		}()
	}
	wg.Wait()
}
func TestScopePositionFailureDoesNotMutateMarks(t *testing.T) {
	g := markerGraph()
	m := NewScopeMarker(g.Base)
	b := src.NewFileBase("f.go", "/src/f.go")
	pos := func(line uint) src.XPos { return g.Base.Ctxt.PosTable.XPos(src.MakePos(b, line, 1)) }
	m.Push(pos(5))
	for _, bad := range []src.XPos{{}, pos(4)} {
		func() {
			defer func() {
				if recover() == nil {
					t.Error("invalid scope position accepted")
				}
			}()
			m.Push(bad)
		}()
	}
	if len(m.parents) != 1 || len(m.marks) != 1 {
		t.Fatal("invalid position changed scopes")
	}
	m.Pop(pos(6))
}

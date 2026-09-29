package position_test

import (
	"fmt"
	"go-backend-ssa/gcbridge"
	"go-backend-ssa/position"
	"sync"
	"testing"
)

func TestNativeBridgeIdentityAndLifetime(t *testing.T) {
	var table position.Table
	base := position.NewFile("input.go", "/input.go")
	line := position.NewLine(position.Make(base, 10, 3), "original.go", "/original.go", 100, 5)
	p := position.Make(line, 12, 7)
	var native gcbridge.Pos = p
	x := gcbridge.Encode(&table, native).WithIsStmt().WithXlogue(position.PrologueEnd)
	got := gcbridge.Decode(&table, x)
	if got.Base() != p.Base() || got.RelLine() != 102 || got.RelCol() != 7 || got.RelFilename() != "original.go" {
		t.Fatalf("position identity or line directive lost: %v", got)
	}
	if got.IsStmt() != position.IsStmt || got.Xlogue() != position.PrologueEnd {
		t.Fatal("debug marks lost")
	}
	for i := 0; i < 30; i++ {
		var other position.Table
		b := position.NewFile("input.go", fmt.Sprintf("/other/%d.go", i))
		other.XPos(position.Make(b, 200, 1))
	}
	if table.Pos(x).Base() != line || table.FileTable()[0] != "/original.go" {
		t.Fatal("later compilation invalidated old result")
	}
	names := table.FileTable()
	names[0] = "changed"
	if table.FileTable()[0] != "/original.go" {
		t.Fatal("file table leaks mutable slice")
	}
	var absent *position.Base
	a, b := absent.Pos(), absent.Pos()
	a.SetBase(base)
	if b.Base() != nil || absent.Pos().Base() != nil {
		t.Fatal("unknown position is shared mutable state")
	}
}

func TestSeparateCompilationsConcurrent(t *testing.T) {
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			var table position.Table
			b := position.NewFile("same.go", fmt.Sprintf("/%d/same.go", i))
			inlined := position.NewInlining(b, i)
			for j := uint(1); j < 100; j++ {
				p := position.Make(inlined, j, 2)
				x := table.XPos(p)
				got := table.Pos(x)
				if got.Base() != inlined || got.Line() != j || got.Base().InliningIndex() != i {
					t.Errorf("cross-compilation state: %v", got)
					return
				}
			}
		}(i)
	}
	wg.Wait()
}

func TestBaseCannotBeRegisteredInAnotherTable(t *testing.T) {
	var a, b position.Table
	p := position.Make(position.NewFile("a.go", "/a.go"), 1, 1)
	a.XPos(p)
	defer func() {
		if recover() == nil {
			t.Error("accepted a base owned by another table")
		}
	}()
	b.XPos(p)
}

package noder_test

import (
	"fmt"
	"go-backend-ssa/gcbridge"
	"go-backend-ssa/noder"
	"go-backend-ssa/position"
	"go-backend-ssa/syntax"
	"strings"
	"sync"
	"testing"
)

func TestParserToNativePositions(t *testing.T) {
	link := &gcbridge.Link{Pathname: "/work/src"}
	m := noder.NewPosMap(link, "/work=>example", "/sdk/go")
	f, err := syntax.Parse(syntax.NewFileBase("p.go"), strings.NewReader("package p\n//line virtual.go:40:3\nvar X int\n"), nil, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	first := m.XPos(f.Pos())
	native := link.PosTable.Pos(first)
	if native.Base().AbsFilename() != "example/src/p.go" || native.RelLine() != 1 {
		t.Fatal(native, native.Base().AbsFilename())
	}
	varPos := f.DeclList[0].Pos()
	mapped := link.PosTable.Pos(m.XPos(varPos))
	if mapped.RelLine() != 40 || mapped.RelCol() != varPos.RelCol() || mapped.RelFilename() != varPos.RelFilename() {
		t.Fatalf("line pragma: %v vs %v", mapped, varPos)
	}
	if m.XPos(f.Pos()) != first {
		t.Fatal("unstable cache")
	}
	if m.XPos(syntax.Pos{}) != (position.XPos{}) {
		t.Fatal("unknown position")
	}
	b := syntax.NewFileBase("/sdk/go/src/runtime/proc.go")
	if got := m.TrimFilename(b); got != "$GOROOT/src/runtime/proc.go" {
		t.Fatal(got)
	}
	trimmed := syntax.NewTrimmedFileBase("already/trimmed.go", true)
	if got := m.TrimFilename(trimmed); got != "already/trimmed.go" {
		t.Fatal(got)
	}
	// Later maps and failures do not clear or remap previously returned XPos.
	other := &gcbridge.Link{Pathname: "/another"}
	noder.NewPosMap(other, "", "").XPos(f.Pos())
	if got := link.PosTable.Pos(first); got != native {
		t.Fatal("old position changed")
	}
}

func TestConcurrentPositionMaps(t *testing.T) {
	base := syntax.NewFileBase("same.go")
	var wg sync.WaitGroup
	for i := range 16 {
		wg.Go(func() {
			dir := fmt.Sprintf("/root/%d", i)
			link := &gcbridge.Link{Pathname: dir}
			m := noder.NewPosMap(link, "", "")
			pos := syntax.MakePos(base, 8, 2)
			for range 20 {
				native := link.PosTable.Pos(m.XPos(pos))
				if native.Base().AbsFilename() != dir+"/same.go" {
					t.Error(native.Base().AbsFilename())
				}
			}
		})
	}
	wg.Wait()
}

package diagnostic_test

import (
	"go-backend-ssa/diagnostic"
	"go-backend-ssa/position"
	"testing"
)

func TestSnapshotDoesNotRetainMutablePositions(t *testing.T) {
	var a, b diagnostic.Collector
	p := position.Make(position.NewFile("a.go", "/a.go"), 3, 4)
	a.Add(diagnostic.Error, "syntax", p, "bad token")
	*p.Base().Pos() = position.Pos{}
	first := a.Diagnostics()
	if first[0].String() != "a.go:3:4: bad token" || !a.HasErrors() {
		t.Fatal(first)
	}
	first[0].Message = "modified"
	a.Add(diagnostic.Warning, "unused", position.Pos{}, "unused")
	if a.Diagnostics()[0].Message != "bad token" || len(b.Diagnostics()) != 0 || b.HasErrors() {
		t.Fatal("diagnostic ownership leak")
	}
}

package consumer

import (
	"go-backend-ssa/diagnostic"
	"go-backend-ssa/gcbridge"
	"go-backend-ssa/position"
	"go-backend-ssa/target"
	"testing"
)

func TestPublicObjects(t *testing.T) {
	layout, err := target.NewLayout("linux", "386")
	if err != nil || layout.PointerSize() != 4 {
		t.Fatal(layout, err)
	}
	var positions position.Table
	p := position.Make(position.NewFile("main.go", "/main.go"), 7, 3)
	var native gcbridge.Pos = p
	var nativeTable *gcbridge.PosTable = &positions
	var compressed position.XPos = gcbridge.Encode(nativeTable, native)
	if gcbridge.Decode(nativeTable, compressed) != p {
		t.Fatal("different object model")
	}
	var diagnostics diagnostic.Collector
	diagnostics.Add(diagnostic.Warning, "probe", p, "example")
	if diagnostics.Diagnostics()[0].String() != "main.go:7:3: example" {
		t.Fatal(diagnostics.Diagnostics())
	}
}

package consumer

import (
	"go-backend-ssa/diagnostic"
	"go-backend-ssa/gcbridge"
	"go-backend-ssa/position"
	"go-backend-ssa/target"
	"go-backend-ssa/types"
	"testing"
)

func TestPublicObjects(t *testing.T) {
	layout, err := target.NewLayout("linux", "386")
	if err != nil || layout.PointerSize() != 4 {
		t.Fatal(layout, err)
	}
	u, err := types.NewUniverse(layout)
	if err != nil {
		t.Fatal(err)
	}
	scalar, err := u.Scalar(types.Int)
	if err != nil {
		t.Fatal(err)
	}
	var nativeType *gcbridge.Type = scalar
	pkg := u.NewPkg("example", "example")
	var nativeSymbol *gcbridge.Sym = pkg.Lookup("Value")
	pkg.Pathsym = &gcbridge.LinkSymbol{Name: "example"}
	if nativeType.Universe() != u || nativeSymbol.Universe() != u || pkg.Pathsym.Name != "example" {
		t.Fatal("type/symbol bridge changed object identity")
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

// Object remains implementable outside the compiler's internal package tree.
type binding struct {
	sym *types.Sym
	typ *types.Type
}

func (b binding) Pos() position.XPos { return position.XPos{} }
func (b binding) Sym() *types.Sym    { return b.sym }
func (b binding) Type() *types.Type  { return b.typ }

var _ types.Object = binding{}
var _ interface{ CanBeAnSSAAux() } = (*types.Type)(nil)

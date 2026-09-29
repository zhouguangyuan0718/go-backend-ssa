package base

import (
	"go-backend-ssa/internal/cmd/obj"
	"go-backend-ssa/internal/cmd/src"
	"testing"
)

func TestNativePositionFormatting(t *testing.T) {
	c := NewLinkContext(&obj.Link{})
	p := c.Ctxt.PosTable.XPos(src.MakePos(src.NewFileBase("p.go", "/src/p.go"), 12, 3))
	if got := c.FmtPos(p); got != "p.go:12:3" {
		t.Fatal(got)
	}
	c.Flag.C = 1
	if got := c.FmtPos(p); got != "p.go:12" {
		t.Fatal(got)
	}
	other := NewLinkContext(&obj.Link{})
	c.ReservedImports["custom"] = true
	if s := c.PkgLinksym("custom", "X", obj.ABI0); s.Name != "custom:X" {
		t.Fatal(s.Name)
	}
	if s := other.PkgLinksym("custom", "X", obj.ABI0); s.Name != "custom.X" {
		t.Fatal("shared reserved imports", s.Name)
	}
}

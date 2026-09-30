package x86

import (
	"bytes"
	"fmt"
	"reflect"
	"testing"

	"go-backend-ssa/internal/cmd/obj"
	"go-backend-ssa/internal/cmd/sys"
)

func testAssembler(t *testing.T, arch string) *Context {
	t.Helper()
	arches := sys.NewContext()
	a := arches.ArchAMD64
	if arch == "386" {
		a = arches.Arch386
	}
	link := &obj.Link{Arch: &obj.LinkArch{Arch: a}, TargetOS: "linux", Pkgpath: "example/p", Flag_optimize: true}
	if err := link.Headtype.Set("linux"); err != nil {
		t.Fatal(err)
	}
	link.DiagFunc = func(f string, args ...any) { panic(fmt.Sprintf(f, args...)) }
	c := NewContext(link)
	link.Arch = &c.Linkamd64
	if arch == "386" {
		link.Arch = &c.Link386
	}
	link.Arch.Init(link)
	return c
}
func testAssemblerFunction(c *Context, name string, value int64) *obj.LSym {
	link := c.Link
	s := link.Lookup(name)
	fi := s.NewFuncInfo()
	text := link.NewProg()
	fi.Text = text
	text.As = obj.ATEXT
	text.From = obj.Addr{Type: obj.TYPE_MEM, Name: obj.NAME_EXTERN, Sym: s}
	text.To = obj.Addr{Type: obj.TYPE_TEXTSIZE, Val: int32(0)}
	s.Set(obj.AttrNoSplit|obj.AttrNoFrame, true)
	mov := link.NewProg()
	mov.As = AMOVQ
	if link.Arch.Family == sys.I386 {
		mov.As = AMOVL
	}
	mov.From = obj.Addr{Type: obj.TYPE_CONST, Offset: value}
	mov.To = obj.Addr{Type: obj.TYPE_REG, Reg: REG_AX}
	ret := link.NewProg()
	ret.As = obj.ARET
	text.Link = mov
	mov.Link = ret
	return s
}
func assembleTestFunction(c *Context, s *obj.LSym) {
	link := c.Link
	for p := s.Func().Text; p != nil; p = p.Link {
		link.Arch.Progedit(link, p, link.NewProg)
	}
	if link.Arch.ErrorCheck != nil {
		link.Arch.ErrorCheck(link, s)
	}
	link.Arch.Preprocess(link, s, link.NewProg)
	link.Arch.Assemble(link, s, link.NewProg)
}
func TestNativeAssemblerIsolation(t *testing.T) {
	for i := range 8 {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			t.Parallel()
			a, b := testAssembler(t, "amd64"), testAssembler(t, "386")
			if a.opindex[AMOVQ&obj.AMask] == b.opindex[AMOVQ&obj.AMask] || a.ycover[0] != 1 || b.ycover[0] != 1 {
				t.Fatal("native table initialization or ownership")
			}
			first := testAssemblerFunction(a, "first", 42)
			assembleTestFunction(a, first)
			saved := bytes.Clone(first.P)
			second := testAssemblerFunction(b, "second", 51)
			assembleTestFunction(b, second)
			if len(saved) == 0 || saved[len(saved)-1] != 0xc3 || len(second.P) == 0 || second.P[len(second.P)-1] != 0xc3 {
				t.Fatal("missing machine code return")
			}
			if bytes.Equal(saved, second.P) || !bytes.Equal(saved, first.P) {
				t.Fatal("target output or retained bytes")
			}
			b.ycover[0] = 9
			if a.ycover[0] != 1 {
				t.Fatal("operand tables shared")
			}
			a.Link.Arch.Init(a.Link)
			again := testAssemblerFunction(a, "again", 42)
			assembleTestFunction(a, again)
			if !bytes.Equal(saved, again.P) {
				t.Fatal("reinitialization or another target changed assembly")
			}
		})
	}
}
func TestAssemblerRejectsForeignInputsBeforeMutation(t *testing.T) {
	a, b := testAssembler(t, "amd64"), testAssembler(t, "amd64")
	own := testAssemblerFunction(a, "own", 42)
	other := testAssemblerFunction(b, "other", 51)
	original := *own.Func().Text.Link
	reject := func(name string, run func()) {
		t.Helper()
		func() {
			defer func() {
				if recover() == nil {
					t.Error(name, "accepted")
				}
			}()
			run()
		}()
		if len(own.P) != 0 || len(other.P) != 0 || !reflect.DeepEqual(*own.Func().Text.Link, original) {
			t.Fatal(name, "mutated input before rejecting")
		}
	}
	reject("foreign init", func() { a.Link.Arch.Init(b.Link) })
	reject("foreign preprocess", func() { a.Link.Arch.Preprocess(a.Link, other, a.Link.NewProg) })
	reject("foreign assemble link", func() { a.Link.Arch.Assemble(b.Link, other, b.Link.NewProg) })
	reject("foreign instruction", func() { a.Link.Arch.Progedit(a.Link, other.Func().Text, a.Link.NewProg) })
	reject("nil allocator", func() { a.Link.Arch.Assemble(a.Link, own, nil) })
	assembleTestFunction(a, own)
	if len(own.P) == 0 {
		t.Fatal("rejected inputs poisoned assembler")
	}
}

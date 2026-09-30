package arm64

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"reflect"
	"testing"

	"go-backend-ssa/internal/cmd/obj"
	"go-backend-ssa/internal/cmd/sys"
	"go-backend-ssa/internal/go/buildcfg"
)

func testAssembler(t *testing.T, simd bool) *Context {
	t.Helper()
	arches := sys.NewContext()
	a := arches.ArchARM64
	link := &obj.Link{Arch: &obj.LinkArch{Arch: a}, TargetOS: "linux", Pkgpath: "example/p", Flag_optimize: true}
	if err := link.Headtype.Set("linux"); err != nil {
		t.Fatal(err)
	}
	link.DiagFunc = func(f string, args ...any) { panic(fmt.Sprintf(f, args...)) }
	experiment := buildcfg.ExperimentFlags{}
	experiment.SIMD = simd
	c := NewContext(link, experiment)
	link.Arch = &c.Linkarm64
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
	mov.As = AMOVD
	mov.From = obj.Addr{Type: obj.TYPE_CONST, Offset: value}
	mov.To = obj.Addr{Type: obj.TYPE_REG, Reg: REG_R0}
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
			a, b := testAssembler(t, false), testAssembler(t, false)
			if len(a.oprange[AMOVD&obj.AMask]) == 0 || &a.oprange[AMOVD&obj.AMask][0] == &b.oprange[AMOVD&obj.AMask][0] {
				t.Fatal("native table initialization or ownership")
			}
			first := testAssemblerFunction(a, "first", 42)
			assembleTestFunction(a, first)
			saved := bytes.Clone(first.P)
			second := testAssemblerFunction(b, "second", 51)
			assembleTestFunction(b, second)
			if len(saved) < 8 || !bytes.Equal(saved[4:8], []byte{0xc0, 0x03, 0x5f, 0xd6}) || len(second.P) < 8 || !bytes.Equal(second.P[4:8], []byte{0xc0, 0x03, 0x5f, 0xd6}) {
				t.Fatal("missing machine code return")
			}
			if bytes.Equal(saved, second.P) || !bytes.Equal(saved, first.P) {
				t.Fatal("target output or retained bytes")
			}
			originalName := a.Anames[0]
			b.Anames[0] = "changed"
			if a.Anames[0] != originalName {
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
	a, b := testAssembler(t, false), testAssembler(t, false)
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

// Expected encodings are from pinned Go 1.27.1 arm64sveenc.s.
func TestSVEEncodingAndOwnedTables(t *testing.T) {
	for i := range 8 {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			t.Parallel()
			a, b := testAssembler(t, true), testAssembler(t, false)
			z := func(r int16) obj.Addr { return obj.Addr{Type: obj.TYPE_REG, Reg: REG_ZARNG | ARNG_D<<5 | r} }
			for _, tc := range []struct {
				op       obj.As
				from, to obj.Addr
				rest     []obj.AddrPos
				want     uint32
			}{
				{op: ASETFFR, want: 0x252c9000},
				{op: AZADD, from: z(7), to: z(13), rest: []obj.AddrPos{{Addr: z(23), Pos: obj.Source}}, want: 0x04e702ed},
			} {
				s := testAssemblerFunction(a, "sve"+fmt.Sprint(tc.op), 42)
				p := s.Func().Text.Link
				p.As = tc.op
				p.From = tc.from
				p.To = tc.to
				p.RestArgs = tc.rest
				assembleTestFunction(a, s)
				if len(s.P) < 4 || binary.LittleEndian.Uint32(s.P) != tc.want {
					t.Fatalf("%v encoding %x want %08x", tc.op, s.P, tc.want)
				}
			}
			idx := int(AZADD - ASVESTART - 1)
			if len(a.insts[idx]) == 0 || len(a.insts[idx][0].args) == 0 || len(a.insts[idx][0].args[0].elemEncoders) == 0 {
				t.Fatal("SVE tables not initialized")
			}
			expected := a.insts[idx][0].fixedBits
			b.insts[idx][0].fixedBits ^= 1
			if a.insts[idx][0].fixedBits != expected {
				t.Fatal("SVE instruction table shared")
			}
			expectedClass := a.insts[idx][0].args[0].class
			b.insts[idx][0].args[0].class = AC_NONE
			if a.insts[idx][0].args[0].class != expectedClass {
				t.Fatal("SVE operand table shared")
			}
			expectedComponent := a.insts[idx][0].args[0].elemEncoders[0].comp
			b.insts[idx][0].args[0].elemEncoders[0].comp ^= 1
			if a.insts[idx][0].args[0].elemEncoders[0].comp != expectedComponent {
				t.Fatal("SVE encoder table shared")
			}
		})
	}
}

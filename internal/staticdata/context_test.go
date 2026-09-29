package staticdata

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"go-backend-ssa/internal/cmd/obj"
	"go-backend-ssa/internal/cmd/src"
	"go-backend-ssa/internal/cmd/sys"
	"go-backend-ssa/internal/ir"
	"go-backend-ssa/internal/objw"
	"go-backend-ssa/internal/typecheck"
	"go-backend-ssa/internal/types"
	"go/constant"
	"strings"
	"sync"
	"testing"
)

func testContext(size int, order binary.ByteOrder) *Context {
	link := &obj.Link{Arch: &obj.LinkArch{Arch: &sys.Arch{Name: "test", PtrSize: size, RegSize: size, ByteOrder: order}}}
	u := types.NewUniverseWithPositions(size, size, 1<<50, &link.PosTable)
	u.LocalPkg = u.NewPkg("example/p", "p")
	graph := ir.NewContext(u, link)
	typecheck.NewContext(graph).InitUniverse()
	return NewContext(graph, objw.NewContext(u, link, "linux"))
}
func global(c *Context, name string, typ *types.Type) *ir.Name {
	n := c.IR.NewNameAt(src.XPos{}, c.IR.Types.LocalPkg.Lookup(name), typ)
	n.Class = ir.PEXTERN
	return n
}
func TestNativeStringsAndInitializers(t *testing.T) {
	for _, size := range []int{4, 8} {
		for _, order := range []binary.ByteOrder{binary.LittleEndian, binary.BigEndian} {
			c := testContext(size, order)
			u := c.IR.Types
			for _, s := range []string{"", "go\x00语言", strings.Repeat("a", 100), strings.Repeat("a", 101)} {
				sym := c.StringSym(src.XPos{}, s)
				count := len(c.Writer.Link.Data)
				if !bytes.Equal(sym.P, []byte(s)) || sym.Size != int64(len(s)) || sym.Align != 1 || sym.Attribute&obj.AttrContentAddressable == 0 || !sym.Local() || !sym.DuplicateOK() || c.StringSym(src.XPos{}, s) != sym || len(c.Writer.Link.Data) != count {
					t.Fatal("native string data", sym.Name)
				}
				if len(s) > 100 && !strings.HasPrefix(sym.Name, "go:string..gostring.101.") {
					t.Fatal("native long string name", sym.Name)
				}
			}
			a, b := c.StringSymNoCommon("method"), c.StringSymNoCommon("method")
			if a == b || a.Attribute&obj.AttrContentAddressable != 0 || b.Attribute&obj.AttrContentAddressable != 0 || string(a.P) != "method" || !a.OnList() {
				t.Fatal("non-common linker string")
			}
			n := global(c, "bytes", u.NewSlice(u.Types[types.TUINT8]))
			c.InitSliceBytes(n, 0, "abc")
			sym := c.GlobalLinksym(n)
			read := func(data []byte) uint64 {
				if size == 4 {
					return uint64(order.Uint32(data))
				}
				return order.Uint64(data)
			}
			if len(sym.R) != 1 || string(sym.R[0].Sym.P) != "abc" || read(sym.P[size:]) != 3 || read(sym.P[2*size:]) != 3 {
				t.Fatal("native slice header")
			}
			other := global(c, "other", n.Type())
			c.InitSliceBytes(other, 0, "abc")
			if other.Linksym().R[0].Sym == sym.R[0].Sym || c.slicedataGen != 2 {
				t.Fatal("mutable backing stores coalesced")
			}
			address := global(c, "address", u.NewPtr(u.Types[types.TUINT8]))
			c.InitAddrOffset(address, 0, sym, 5)
			if len(address.Linksym().R) != 1 || address.Linksym().R[0].Add != 5 || address.Linksym().R[0].Sym != sym {
				t.Fatal("address relocation")
			}
			c.InitConst(global(c, "integer", u.Types[types.TINT64]), 0, c.IR.NewBasicLit(src.XPos{}, u.Types[types.TINT64], constant.MakeInt64(-7)), 8)
			if order.Uint64(u.LocalPkg.Lookup("integer").Linksym().P) != ^uint64(6) {
				t.Fatal("integer constant")
			}
			str := global(c, "string", u.Types[types.TSTRING])
			c.InitConst(str, 0, c.IR.NewBasicLit(src.XPos{}, str.Type(), constant.MakeString("abc")), 2*size)
			if read(str.Linksym().P[size:]) != 3 || str.Linksym().R[0].Sym != c.StringSym(src.XPos{}, "abc") {
				t.Fatal("string constant")
			}
		}
	}
}
func TestConcurrentFunctionRegistration(t *testing.T) {
	c := testContext(8, binary.LittleEndian)
	u := c.IR.Types
	var names []*ir.Name
	for _, name := range []string{"Z", "A", "M"} {
		names = append(names, c.IR.NewFunc(src.XPos{}, src.XPos{}, u.LocalPkg.Lookup(name), u.NewSignature(nil, nil, nil)).Nname)
	}
	var wg sync.WaitGroup
	got := make([]*obj.LSym, 96)
	for i := range got {
		wg.Go(func() { got[i] = c.FuncLinksym(names[i%3]) })
	}
	wg.Wait()
	if len(c.funcsyms) != 3 {
		t.Fatal("duplicate registrations", len(c.funcsyms))
	}
	for i, s := range got {
		if s != got[i%3] || s.OnList() {
			t.Fatal("registration identity or premature write")
		}
	}
	c.WriteFuncSyms()
	if len(c.Writer.Link.Data) != 3 {
		t.Fatal("function-value emission")
	}
	for i, s := range c.Writer.Link.Data {
		if len(s.R) != 1 || s.R[0].Sym.ABI() != obj.ABIInternal || s.R[0].Siz != 8 || s.Size != 8 || !s.DuplicateOK() {
			t.Fatal("function closure relocation")
		}
		if i > 0 && c.Writer.Link.Data[i-1].Name >= s.Name {
			t.Fatal("native symbol sorting")
		}
	}
	// Native runtime special case tolerates values already emitted by the driver.
	c.IR.Base.Flag.CompilingRuntime = true
	c.WriteFuncSyms()
	if len(c.Writer.Link.Data) != 3 {
		t.Fatal("runtime re-emission")
	}
}
func TestIndependentStaticDataAndRejectedInputs(t *testing.T) {
	var wg sync.WaitGroup
	for i := range 12 {
		wg.Go(func() {
			c := testContext(4+4*(i%2), binary.LittleEndian)
			u := c.IR.Types
			n := global(c, "bytes", u.NewSlice(u.Types[types.TUINT8]))
			c.InitSliceBytes(n, 0, "first")
			retained := bytes.Clone(n.Linksym().P)
			backing := n.Linksym().R[0].Sym
			d := testContext(12-u.PointerSize(), binary.BigEndian)
			du := d.IR.Types
			other := global(d, "bytes", du.NewSlice(du.Types[types.TUINT8]))
			d.InitSliceBytes(other, 0, "second")
			if c.slicedataGen != 1 || d.slicedataGen != 1 || backing == other.Linksym().R[0].Sym || string(backing.P) != "first" || !bytes.Equal(retained, n.Linksym().P) {
				t.Error("independent data state")
			}
			for _, fn := range []func(){func() { NewContext(c.IR, d.Writer) }, func() { c.GlobalLinksym(other) }, func() { c.InitSliceBytes(other, 0, "bad") }, func() {
				c.InitConst(n, 0, d.IR.NewBasicLit(src.XPos{}, du.Types[types.TINT], constant.MakeInt64(1)), 4)
			}, func() { c.FuncLinksym(n) }} {
				mustPanic(t, fn)
			}
			if d.slicedataGen != 1 || c.slicedataGen != 1 {
				t.Error("rejected request changed counter")
			}
			// Diagnostic adapter uses the existing IR collector and position table.
			pos := c.Writer.Link.PosTable.XPos(src.MakePos(src.NewFileBase("p.go", "p.go"), 7, 1))
			c.errorfAt(pos, 0, "constant %d", i)
			if got := fmt.Sprint(c.IR.Diagnostics.Diagnostics()); !strings.Contains(got, fmt.Sprint("constant ", i)) {
				t.Error("shared diagnostic collector", got)
			}
		})
	}
	wg.Wait()
}
func mustPanic(t *testing.T, fn func()) {
	t.Helper()
	defer func() {
		if recover() == nil {
			t.Error("expected rejection")
		}
	}()
	fn()
}

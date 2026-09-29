package rttype

import (
	"bytes"
	"encoding/binary"
	"go-backend-ssa/internal/cmd/obj"
	"go-backend-ssa/internal/cmd/objabi"
	"go-backend-ssa/internal/cmd/sys"
	"go-backend-ssa/internal/ir"
	"go-backend-ssa/internal/objw"
	"go-backend-ssa/internal/typecheck"
	"go-backend-ssa/internal/types"
	"reflect"
	"sync"
	"testing"
)

func testContext(size int, order binary.ByteOrder) *Context {
	link := &obj.Link{Arch: &obj.LinkArch{Arch: &sys.Arch{Name: "test", PtrSize: size, RegSize: size, ByteOrder: order}}}
	u := types.NewUniverseWithPositions(size, size, 1<<50, &link.PosTable)
	typecheck.NewContext(ir.NewContext(u, link)).InitUniverse()
	c := NewContext(objw.NewContext(u, link, "linux"))
	c.Init()
	return c
}
func mustPanic(t *testing.T, fn func()) {
	t.Helper()
	defer func() {
		if recover() == nil {
			t.Error("expected rejected operation")
		}
	}()
	fn()
}
func TestRuntimeLayoutsAndRetainedCursors(t *testing.T) {
	var wg sync.WaitGroup
	for i := range 16 {
		wg.Go(func() {
			size := 4 + 4*(i%2)
			order := binary.ByteOrder(binary.LittleEndian)
			if i%4 >= 2 {
				order = binary.BigEndian
			}
			c := testContext(size, order)
			if c.Type.Size() != int64(4*size+16) || c.StructField.Size() != int64(3*size) || c.ITab.OffsetOf("Type") != int64(size) {
				t.Error("runtime target layout")
			}
			original := c.ITab
			symbol := c.Writer.Link.Lookup("p.itab")
			target := c.Writer.Link.Lookup("p.fn")
			cur := c.NewCursor(symbol, 0, original)
			cur.Field("Hash").WriteUint32(0x12345678)
			fun, delta := cur.Field("Fun").ModifyArray(3)
			if delta != int64(2*size) || original.Field(3).Type.NumElem() != 1 {
				t.Error("virtual array changed native layout")
			}
			fun.Elem(2).WritePtrWeak(target)
			if len(symbol.R) != 1 || symbol.R[0].Type != objabi.R_WEAKADDR || symbol.R[0].Siz != uint8(size) || symbol.R[0].Off != int32(original.OffsetOf("Fun")+2*int64(size)) {
				t.Error("target relocation")
			}
			oldBytes := bytes.Clone(symbol.P)
			oldSize := original.Size()
			newer := testContext(12-size, binary.LittleEndian)
			if newer.Type == c.Type || newer.ITab == original || original.Size() != oldSize || !bytes.Equal(oldBytes, symbol.P) {
				t.Error("new compilation changed old runtime graph")
			}
			// Continue writing through a retained cursor after constructing another instance.
			cur.Field("Hash").WriteUint32(0xdeadbeef)
			if order.Uint32(symbol.P[original.OffsetOf("Hash"):]) != 0xdeadbeef {
				t.Error("cursor lost target")
			}
			for _, typ := range []*types.Type{c.Type, c.ITab, c.InterfaceSwitch, c.TypeAssert} {
				for _, f := range typ.Fields() {
					if f.Sym.Universe() != c.Types || f.Type.Universe() != c.Types {
						t.Error("runtime field escaped compilation")
					}
				}
			}
		})
	}
	wg.Wait()
}
func TestRuntimeCursorBoundaries(t *testing.T) {
	c := testContext(8, binary.LittleEndian)
	other := testContext(4, binary.BigEndian)
	s := c.Writer.Link.Lookup("p.data")
	cur := c.NewCursor(s, 0, c.ITab)
	for _, fn := range []func(){
		c.Init,
		func() { c.NewCursor(s, 0, other.Type) },
		func() { c.NewArrayCursor(s, 0, other.Type, 2) },
		func() { c.NewCursor(nil, 0, c.Type) },
		func() { cur.Field("Missing") },
		func() { cur.Field("Hash").WritePtr(nil) },
		func() { cur.Field("Fun").Elem(-1) },
		func() { cur.Field("Fun").Elem(1) },
		func() { cur.Elem(0) },
		func() { a, _ := cur.Field("Fun").ModifyArray(2); a.Elem(2) },
		func() { var zero Cursor; zero.WriteBool(true) },
		func() { var zero ArrayCursor; zero.Elem(0) },
		func() { c.FromReflect(reflect.TypeFor[map[string]int]()) },
	} {
		mustPanic(t, fn)
	}
	if len(s.P) != 0 || len(s.R) != 0 {
		t.Fatal("rejected operation changed object data")
	}
	if c.ITab.Size() != 32 || other.ITab.Size() != 16 {
		t.Fatal("rejected reinitialization changed layouts")
	}
}

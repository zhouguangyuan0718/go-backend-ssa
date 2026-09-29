package reflectdata

import (
	"bytes"
	"go-backend-ssa/internal/bitvec"
	"go-backend-ssa/internal/cmd/obj"
	"go-backend-ssa/internal/cmd/objabi"
	"go-backend-ssa/internal/cmd/src"
	"go-backend-ssa/internal/ir"
	"go-backend-ssa/internal/typebits"
	"go-backend-ssa/internal/types"
	"sync"
	"testing"
)

func TestNativeGCBitmapsAndDeferredEmission(t *testing.T) {
	for _, size := range []int{4, 8} {
		c := testContext(size, false)
		u := c.Typecheck.IR.Types
		pointer := u.NewPtr(u.Types[types.TINT])
		iface := u.NewInterface(nil)
		record := u.NewStruct([]*types.Field{u.NewField(src.XPos{}, nil, pointer), u.NewField(src.XPos{}, nil, u.Types[types.TINT]), u.NewField(src.XPos{}, nil, iface), u.NewField(src.XPos{}, nil, u.NewSlice(pointer))})
		u.CalcSize(record)
		// Pointer at slot 0, interface data at slot 3, slice backing store at slot 4.
		bitmap := bitvec.New(int32(record.Size() / int64(size)))
		typebits.Set(record, 0, bitmap)
		if bitmap.Count() != 3 || !bitmap.Get(0) || bitmap.Get(2) || !bitmap.Get(3) || !bitmap.Get(4) {
			t.Fatal("GC pointer slots", bitmap)
		}
		sym, ptrdata := c.GCSym(record, false)
		if ptrdata != int64(5*size) || sym.OnList() || len(sym.P) != 0 || len(c.gcsymset) != 1 {
			t.Fatal("premature GC symbol emission", ptrdata)
		}
		c.WriteGCSymbols()
		want := make([]byte, size)
		want[0] = 0x19
		if !bytes.Equal(sym.P, want) || sym.Type != objabi.SRODATA || sym.Align != int16(size) || !sym.Local() || !sym.DuplicateOK() {
			t.Fatal("native mask bytes/flags", sym)
		}
		count := len(c.Runtime.Writer.Link.Data)
		c.WriteGCSymbols()
		if len(c.Runtime.Writer.Link.Data) != count {
			t.Fatal("duplicate GC symbol emission")
		}
		other := testContext(12-size, false)
		other.GCSym(other.Typecheck.IR.Types.NewPtr(other.Typecheck.IR.Types.Types[types.TINT]), false)
		other.WriteGCSymbols()
		if !bytes.Equal(sym.P, want) || sym.Align != int16(size) {
			t.Fatal("new instance changed retained GC symbol")
		}
	}
}
func TestGCOnDemandThresholdAndZeroAddress(t *testing.T) {
	c := testContext(8, false)
	u := c.Typecheck.IR.Types
	pointer := u.NewPtr(u.Types[types.TINT])
	boundary := u.NewArray(pointer, 128)
	large := u.NewArray(pointer, 129)
	u.CalcSize(boundary)
	u.CalcSize(large)
	normal, demand, n := c.dgcsym(boundary, true, true)
	if demand || n != 1024 || len(normal.P) != 16 {
		t.Fatal("mask threshold")
	}
	pending, n := c.GCSym(large, true)
	if n != 1032 || pending.OnList() {
		t.Fatal("on-demand registration")
	}
	c.WriteGCSymbols()
	// Native WriteGCSymbols writes complete masks even for types requested with
	// onDemandAllowed; descriptor generation separately emits the BSS mask cell.
	fallback, _ := c.GCSym(large, false)
	if !fallback.OnList() || len(fallback.P) != 24 || pending.OnList() {
		t.Fatal("deferred mask policy")
	}
	cell, demand, n := c.dgcsym(large, true, true)
	if cell != pending || !demand || n != 1032 || cell.Type != objabi.SNOPTRBSS || cell.Size != 8 || len(cell.P) != 0 {
		t.Fatal("on-demand mask cell", cell)
	}
	a := c.ZeroAddr(8).(*ir.AddrExpr)
	symbol := a.X.(*ir.LinksymOffsetExpr).Linksym
	for _, size := range []int64{1, 128, 64} {
		if c.ZeroAddr(size).(*ir.AddrExpr).X.(*ir.LinksymOffsetExpr).Linksym != symbol {
			t.Fatal("zero symbol identity")
		}
	}
	if c.ZeroSize != 128 || symbol.Name != "go:map.zero" {
		t.Fatal("zero extent", c.ZeroSize, symbol.Name)
	}
	mustPanic(t, func() { c.ZeroAddr(1 << 31) })
	if c.ZeroSize != 128 {
		t.Fatal("invalid zero extent changed state")
	}
}
func TestConcurrentGCRegistrationUsesFrozenTypeLayouts(t *testing.T) {
	c := testContext(8, false)
	u := c.Typecheck.IR.Types
	p := u.NewPtr(u.Types[types.TINT])
	inputs := []*types.Type{p, u.NewArray(p, 4), u.NewArray(p, 129), u.NewSlice(p)}
	// Native backend concurrency requires completed layouts. No other phase may
	// mutate these graphs while GC symbols are being requested.
	for _, typ := range inputs {
		u.CalcSize(typ)
		u.TypeSymName(typ)
	}
	result := make([]*obj.LSym, 64)
	var wg sync.WaitGroup
	for i := range result {
		wg.Go(func() { result[i], _ = c.GCSym(inputs[i%len(inputs)], true) })
	}
	wg.Wait()
	for i, sym := range result {
		if sym != result[i%len(inputs)] || sym.OnList() {
			t.Fatal("concurrent native symbol identity")
		}
	}
	if len(c.gcsymset) != len(inputs) {
		t.Fatal("lost GC registration")
	}
	c.WriteGCSymbols()
	for _, typ := range inputs {
		sym, _ := c.GCSym(typ, false)
		if !sym.OnList() {
			t.Fatal("pending bitmap not written")
		}
	}
	foreign := testContext(4, false)
	count := len(c.gcsymset)
	mustPanic(t, func() { c.GCSym(foreign.Typecheck.IR.Types.Types[types.TINT], false) })
	if len(c.gcsymset) != count {
		t.Fatal("foreign registration changed queue")
	}
}

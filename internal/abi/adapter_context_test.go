package abi

import (
	"encoding/binary"
	"fmt"
	"go-backend-ssa/internal/cmd/obj"
	"go-backend-ssa/internal/cmd/sys"
	"go-backend-ssa/internal/ir"
	"go-backend-ssa/internal/typecheck"
	"go-backend-ssa/internal/types"
	"reflect"
	"sync"
	"testing"
)

func abiContext(size int) *Context {
	link := &obj.Link{Arch: &obj.LinkArch{Arch: &sys.Arch{Name: "test", PtrSize: size, RegSize: size, ByteOrder: binary.LittleEndian}}}
	u := types.NewUniverseWithPositions(size, size, 1<<50, &link.PosTable)
	u.LocalPkg = u.NewPkg("example/p", "p")
	g := ir.NewContext(u, link)
	typecheck.NewContext(g).InitUniverse()
	return NewContext(g)
}
func TestNativeABIAllocation(t *testing.T) {
	c := abiContext(8)
	u := c.IR.Types
	cfg := c.NewABIConfig(2, 1, 16, 1)
	pair := u.NewStruct([]*types.Field{u.NewField(u.Pos, u.LocalPkg.Lookup("A"), u.Types[types.TINT]), u.NewField(u.Pos, u.LocalPkg.Lookup("B"), u.Types[types.TINT])})
	// The aggregate cannot consume the sole remaining integer register. A later
	// scalar must still get it; input and output budgets are independent.
	ts := []*types.Type{u.Types[types.TINT], pair, u.Types[types.TINT], u.Types[types.TFLOAT64]}
	info := cfg.ABIAnalyzeTypes(ts, ts)
	want := [][]RegIndex{{0}, nil, {1}, {2}}
	for _, ps := range [][]ABIParamAssignment{info.InParams(), info.OutParams()} {
		for i, p := range ps {
			if !reflect.DeepEqual(p.Registers, want[i]) {
				t.Fatalf("parameter %d: %v", i, p.Registers)
			}
		}
	}
	if info.InParam(1).Offset() != 16 || info.OutParam(1).Offset() != 32 || info.SpillAreaOffset() != 48 || info.SpillAreaSize() != 24 || info.ArgWidth() != 56 {
		t.Fatal(info)
	}
	if info.InParam(0).FrameOffset(info) != 32 || info.InParam(1).FrameOffset(info) != 0 {
		t.Fatal("native frame offsets", info)
	}
	if info.InRegistersUsed() != 3 || info.OutRegistersUsed() != 3 || cfg.FloatIndexFor(2) != 0 {
		t.Fatal("register counts")
	}
	if got := c.RegisterTypes(info.InParams()); len(got) != 3 || got[0] != u.Types[types.TINT] || got[2] != u.Types[types.TFLOAT64] {
		t.Fatal(got)
	}
	empty := cfg.ABIAnalyzeTypes([]*types.Type{u.NewStruct(nil), u.NewArray(u.Types[types.TINT], 0)}, nil)
	if len(c.RegisterTypes(empty.InParams())) != 0 || len(empty.InParam(0).Registers) != 0 {
		t.Fatal("zero-size params must use stack")
	}
}
func TestABIComponentsAndPadding(t *testing.T) {
	c := abiContext(8)
	u := c.IR.Types
	fields := []*types.Field{u.NewField(u.Pos, u.LocalPkg.Lookup("a"), u.Types[types.TUINT16]), u.NewField(u.Pos, u.LocalPkg.Lookup("b"), u.Types[types.TUINT8]), u.NewField(u.Pos, u.LocalPkg.Lookup("c"), u.Types[types.TINT32]), u.NewField(u.Pos, u.LocalPkg.Lookup("d"), u.Types[types.TINT32])}
	typ := u.NewStruct(fields)
	info := c.NewABIConfig(8, 8, 0, 1).ABIAnalyzeTypes([]*types.Type{typ, u.Types[types.TSTRING], u.NewSlice(u.Types[types.TINT]), u.Types[types.TINTER]}, nil)
	p := info.InParam(0)
	_, offsets := p.RegisterTypesAndOffsets()
	if !reflect.DeepEqual(offsets, []int64{0, 2, 4, 8}) || !reflect.DeepEqual(p.ComputePadding(make([]uint64, 8)), []uint64{0, 1, 0, 0}) {
		t.Fatal("padding", offsets)
	}
	// Separate analyses provide enough registers for every synthetic component.
	for _, typ := range []*types.Type{u.Types[types.TSTRING], u.NewSlice(u.Types[types.TINT]), u.Types[types.TINTER]} {
		i := c.NewABIConfig(8, 8, 0, 1).ABIAnalyzeTypes([]*types.Type{typ}, nil)
		ts, offs := i.InParam(0).RegisterTypesAndOffsets()
		want := int(typ.Size() / 8)
		if len(ts) != want || len(offs) != want {
			t.Fatal(typ, ts, offs)
		}
		for k, t1 := range ts {
			if t1.Universe() != u || offs[k] != int64(8*k) {
				t.Fatal("synthetic component owner/layout")
			}
		}
	}
}
func TestABIIsolation(t *testing.T) {
	old := abiContext(4)
	u := old.IR.Types
	typ := u.NewSlice(u.Types[types.TINT])
	info := old.NewABIConfig(8, 8, 0, 1).ABIAnalyzeTypes([]*types.Type{typ, u.Types[types.TINT64]}, nil)
	before := info.String()
	oldSynth := old.synthSlice
	ts, offs := info.InParam(1).RegisterTypesAndOffsets()
	if len(ts) != 2 || ts[0] != u.Types[types.TINT32] || ts[1] != u.Types[types.TUINT32] || !reflect.DeepEqual(offs, []int64{0, 4}) {
		t.Fatal("32-bit split", ts, offs)
	}
	var wg sync.WaitGroup
	for i := range 8 {
		wg.Go(func() {
			c := abiContext(8)
			t1 := c.IR.Types.NewSlice(c.IR.Types.Types[types.TINT])
			r := c.NewABIConfig(8, 8, int64(8*i), 1).ABIAnalyzeTypes([]*types.Type{t1}, nil)
			r.InParam(0).RegisterTypesAndOffsets()
			if c.synthSlice == oldSynth || c.synthSlice.Size() != 24 || oldSynth.Size() != 12 || info.String() != before {
				t.Error("cross-compilation ABI cache")
			}
		})
	}
	wg.Wait()
	if old.synthSlice != oldSynth {
		t.Fatal("retained synthetic type replaced")
	}
}
func mustPanic(t *testing.T, f func()) {
	t.Helper()
	defer func() {
		if recover() == nil {
			t.Error("expected ownership rejection")
		}
	}()
	f()
}
func TestABIRejectsForeignInputs(t *testing.T) {
	a, b := abiContext(8), abiContext(4)
	u := a.IR.Types
	cfg := a.NewABIConfig(2, 2, 0, 1)
	mustPanic(t, func() { cfg.ABIAnalyzeTypes([]*types.Type{u.Types[types.TINT], b.IR.Types.Types[types.TINT]}, nil) })
	if a.synthSlice != nil {
		t.Fatal("setup mutated compilation before rejecting all types")
	}
	field := func(c *Context, name string) *types.Field {
		u := c.IR.Types
		f := u.NewField(u.Pos, u.LocalPkg.Lookup(name), u.Types[types.TINT])
		n := c.IR.NewNameAt(u.Pos, f.Sym, f.Type)
		n.Class = ir.PPARAM
		n.SetFrameOffset(123)
		f.Nname = n
		return f
	}
	f, g := field(a, "first"), field(a, "second")
	sig := u.NewSignature(nil, []*types.Field{f, g}, nil)
	g.Nname = field(b, "foreign").Nname
	mustPanic(t, func() { cfg.ABIAnalyze(sig, true) })
	if f.Nname.(*ir.Name).FrameOffset() != 123 || a.synthSlice != nil {
		t.Fatal("mutation before signature rejection")
	}
	ra := cfg.ABIAnalyzeTypes([]*types.Type{u.Types[types.TINT]}, nil)
	rb := b.NewABIConfig(2, 2, 0, 1).ABIAnalyzeTypes([]*types.Type{b.IR.Types.Types[types.TINT]}, nil)
	mustPanic(t, func() { a.RegisterTypes(rb.InParams()) })
	mustPanic(t, func() { ra.InParam(0).FrameOffset(rb) })
	copy := *ra.InParam(0)
	copy.Type = b.IR.Types.Types[types.TINT]
	mustPanic(t, func() { copy.RegisterTypesAndOffsets() })
	_ = fmt.Sprint(ra)
}

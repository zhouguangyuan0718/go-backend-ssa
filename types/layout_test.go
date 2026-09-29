package types_test

import (
	"fmt"
	"go-backend-ssa/position"
	"go-backend-ssa/target"
	"go-backend-ssa/types"
	"sync"
	"testing"
)

type layoutName struct {
	sym *types.Sym
	typ *types.Type
	pos position.XPos
}

func (n *layoutName) Sym() *types.Sym    { return n.sym }
func (n *layoutName) Type() *types.Type  { return n.typ }
func (n *layoutName) Pos() position.XPos { return n.pos }
func defineLayoutName(sym *types.Sym, typ *types.Type) types.Object {
	return &layoutName{sym: sym, typ: typ}
}
func layoutUniverse(t *testing.T, arch string) *types.Universe {
	t.Helper()
	layout, err := target.NewLayout("linux", arch)
	if err != nil {
		t.Fatal(err)
	}
	u, err := types.NewUniverse(layout)
	if err != nil {
		t.Fatal(err)
	}
	u.InitTypes(defineLayoutName)
	return u
}
func scalar(t *testing.T, u *types.Universe, k types.Kind) *types.Type {
	t.Helper()
	v, e := u.Scalar(k)
	if e != nil {
		t.Fatal(e)
	}
	return v
}

func TestNativeLayoutAndAliases(t *testing.T) {
	for _, arch := range []string{"386", "amd64", "arm64"} {
		t.Run(arch, func(t *testing.T) {
			u := layoutUniverse(t, arch)
			p := int64(u.PointerSize())
			i := scalar(t, u, types.Int)
			b := scalar(t, u, types.Uint8)
			if i.Size() != p || i.Alignment() != p || u.SliceSize != 3*p || u.StringSize != 2*p {
				t.Fatal("target layout", i.Size(), u.SliceSize, u.StringSize)
			}
			if !u.Identical(u.ByteType, b) || !u.Identical(u.RuneType, scalar(t, u, types.Int32)) || u.ByteType == b || u.ByteType.String() != "byte" || u.ByteType.LinkString() != b.LinkString() {
				t.Fatal("builtin aliases")
			}
			if !u.Identical(u.AnyType, u.NewInterface(nil)) || !u.Identical(nil, nil) || u.Identical(nil, i) {
				t.Fatal("interface identity or nil identity")
			}
			u.CalcSize(nil)
			u.CheckSize(nil)
			pkg := u.NewPkg("example/layout", "layout")
			makeField := func(name string, typ *types.Type) *types.Field {
				return u.NewField(position.XPos{}, pkg.Lookup(name), typ)
			}
			record := u.NewStruct([]*types.Field{makeField("A", b), makeField("P", u.NewPtr(i)), makeField("Z", u.NewStruct(nil))})
			if record.Size() != 3*p || record.Field(1).Offset != p || record.Field(2).Offset != 2*p || u.PtrDataSize(record) != 2*p || u.AlgType(record) != types.ASPECIAL {
				t.Fatal("struct padding/pointers", record, record.Size(), u.PtrDataSize(record))
			}
			a := u.NewArray(record, 3)
			if a.Size() != 9*p || u.PtrDataSize(a) != 8*p {
				t.Fatal("array pointer prefix")
			}
			ir, fr := a.Registers()
			if ir != 255 || fr != 255 {
				t.Fatal("array register assignment", ir, fr)
			}
			if !u.IsComparable(record) || u.IsComparable(u.NewSlice(i)) || u.AlgType(u.NewInterface(nil)) != types.ANILINTER {
				t.Fatal("comparison algorithm")
			}
			blocked := u.NewStruct([]*types.Field{makeField("S", u.NewSlice(i))})
			if u.IsComparable(blocked) || u.IncomparableField(blocked) != blocked.Field(0) {
				t.Fatal("incomparable field")
			}
			u.DeferCheckSize()
			u.DeferCheckSize()
			pending := u.NewArray(i, 3)
			u.CheckSize(pending)
			u.CheckSize(pending)
			if !pending.Deferwidth() {
				t.Fatal("queue flag")
			}
			u.ResumeCheckSize()
			if !pending.Deferwidth() {
				t.Fatal("nested queue drained early")
			}
			u.ResumeCheckSize()
			if pending.Deferwidth() || pending.Size() != 3*p {
				t.Fatal("queue not drained")
			}
			// Layout computation may be frozen after frontend preparation.
			u.CalcSizeDisabled = true
			if record.Size() != 3*p {
				t.Fatal("cached layout unavailable")
			}
			expectLayoutPanic(t, func() { u.NewArray(i, 4).Size() })
			u.CalcSizeDisabled = false
			// A named recursive type must retain its identity when completed.
			obj := &layoutName{sym: pkg.Lookup("Node")}
			node := u.NewNamed(obj)
			obj.typ = node
			u.DeferCheckSize()
			node.SetUnderlying(u.NewStruct([]*types.Field{makeField("Next", u.NewPtr(node)), makeField("V", i)}))
			u.CheckSize(node)
			u.ResumeCheckSize()
			if node.Size() != 2*p || node.FieldType(0).Elem() != node || node.Obj() != obj {
				t.Fatal("recursive layout")
			}
			sig := u.NewSignature(nil, []*types.Field{makeField("X", node)}, []*types.Field{makeField("Y", i)})
			u.CalcSize(sig)
			if sig.ArgWidth() != 3*p || sig.Size() != p {
				t.Fatal("function layout", sig.ArgWidth())
			}
			// Interface expansion deduplicates matching embedded methods.
			method := makeField("M", u.NewSignature(nil, nil, nil))
			iface := u.NewInterface([]*types.Field{method})
			outer := u.NewInterface([]*types.Field{u.NewField(position.XPos{}, nil, iface), u.NewField(position.XPos{}, nil, iface)})
			if outer.Size() != 2*p || len(outer.AllMethods()) != 1 || u.AlgType(outer) != types.AINTER {
				t.Fatal("method expansion")
			}
			if u.Diagnostics.HasErrors() {
				t.Fatal(u.Diagnostics.Diagnostics())
			}
		})
	}
}
func expectLayoutPanic(t *testing.T, f func()) {
	t.Helper()
	defer func() {
		if recover() == nil {
			t.Error("expected invalid entry to fail")
		}
	}()
	f()
}

func TestLayoutDiagnosticsAndInstanceIsolation(t *testing.T) {
	a, b := layoutUniverse(t, "386"), layoutUniverse(t, "amd64")
	pos := a.PosTable.XPos(position.Make(position.NewFile("bad.go", "/bad.go"), 12, 4))
	a.Pos = pos
	huge := a.NewArray(scalar(t, a, types.Uint8), 1<<16)
	a.CalcSize(a.NewChan(huge, types.SendRecv))
	got := a.Diagnostics.Diagnostics()
	if len(got) != 1 || got[0].Message != "channel element type too large (>64kB)" || got[0].Filename != "bad.go" || got[0].Line != 12 || b.Diagnostics.HasErrors() {
		t.Fatal(got)
	}
	// ErrorfAt keeps the upstream code and formats types using native %L support.
	pos2 := a.PosTable.XPos(position.Make(position.NewFile("dup.go", "/dup.go"), 5, 2))
	pkg := a.NewPkg("dup", "dup")
	sig := a.NewSignature(nil, nil, nil)
	a.CalcSize(a.NewInterface([]*types.Field{a.NewField(pos2, pkg.Lookup("M"), sig), a.NewField(pos2, pkg.Lookup("M"), sig)}))
	got = a.Diagnostics.Diagnostics()
	if len(got) != 2 || got[1].Message != "duplicate method M" || got[1].Filename != "dup.go" || got[1].Code == "0" {
		t.Fatal(got)
	}
	a.CalcSize(a.NewArray(scalar(t, a, types.Uint64), 1<<32))
	got = a.Diagnostics.Diagnostics()
	if got[len(got)-2].Message != "type [4294967296]uint64 larger than address space" {
		t.Fatal(got)
	}
	if a.Pos != pos {
		t.Fatal("diagnostic position not restored")
	}
	expectLayoutPanic(t, func() { a.CalcSize(scalar(t, b, types.Int)) })
	expectLayoutPanic(t, func() { a.Identical(a.Types[types.Int], b.Types[types.Int]) })
	expectLayoutPanic(t, func() { a.InitTypes(defineLayoutName) })
	if scalar(t, b, types.Int).Size() != 8 || b.Diagnostics.HasErrors() {
		t.Fatal("second instance polluted")
	}
}

func TestReentrantAndConcurrentInitialization(t *testing.T) {
	layout, _ := target.NewLayout("linux", "386")
	a, _ := types.NewUniverse(layout)
	layout, _ = target.NewLayout("linux", "amd64")
	b, _ := types.NewUniverse(layout)
	once := false
	a.InitTypes(func(sym *types.Sym, typ *types.Type) types.Object {
		if !once {
			once = true
			b.InitTypes(defineLayoutName)
		}
		if sym.Universe() != a || typ.Universe() != a {
			t.Fatal("callback owner lost")
		}
		return defineLayoutName(sym, typ)
	})
	old := a.NewArray(scalar(t, a, types.Int), 3)
	if old.Size() != 12 || scalar(t, b, types.Int).Size() != 8 {
		t.Fatal("reentrant initialization")
	}
	var wg sync.WaitGroup
	for n := range 24 {
		wg.Go(func() {
			arch := "amd64"
			if n%2 == 0 {
				arch = "386"
			}
			u := layoutUniverse(t, arch)
			v := scalar(t, u, types.Int)
			for range 20 {
				x := u.NewStruct([]*types.Field{u.NewField(position.XPos{}, nil, v)})
				if x.Size() != int64(u.PointerSize()) || fmt.Sprint(x) != "struct { int }" {
					t.Error("parallel layout or formatting", x)
				}
			}
		})
	}
	wg.Wait()
	if old.Size() != 12 || old.Elem().Universe() != a {
		t.Fatal("old graph changed")
	}
}

func TestNilTypeFormatting(t *testing.T) {
	var typ *types.Type
	if typ.String() != "<T>" || typ.LinkString() != "<T>" || typ.NameString() != "<T>" || fmt.Sprintf("%L", typ) != "<T>" {
		t.Fatal("nil type formatting changed")
	}
}

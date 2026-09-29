package reflectdata

import (
	"encoding/binary"
	"fmt"
	"go-backend-ssa/internal/cmd/obj"
	"go-backend-ssa/internal/cmd/objabi"
	"go-backend-ssa/internal/cmd/src"
	"go-backend-ssa/internal/cmd/sys"
	"go-backend-ssa/internal/ir"
	"go-backend-ssa/internal/objw"
	"go-backend-ssa/internal/rttype"
	"go-backend-ssa/internal/staticdata"
	"go-backend-ssa/internal/typecheck"
	"go-backend-ssa/internal/types"
	"sync"
	"testing"
)

func testContext(size int, split bool) *Context {
	link := &obj.Link{Arch: &obj.LinkArch{Arch: &sys.Arch{Name: "test", PtrSize: size, RegSize: size, ByteOrder: binary.LittleEndian}}}
	u := types.NewUniverseWithPositions(size, size, 1<<50, &link.PosTable)
	u.LocalPkg = u.NewPkg("example/p", "p")
	u.Experiment.MapSplitGroup = split
	tc := typecheck.NewContext(ir.NewContext(u, link))
	tc.InitUniverse()
	rt := rttype.NewContext(objw.NewContext(u, link, "linux"))
	rt.Init()
	return NewContext(tc, rt, staticdata.NewContext(tc.IR, rt.Writer))
}
func mustPanic(t *testing.T, fn func()) {
	t.Helper()
	defer func() {
		if recover() == nil {
			t.Error("expected panic")
		}
	}()
	fn()
}
func TestIndependentRuntimeTypeQueuesAndMapCaches(t *testing.T) {
	var wg sync.WaitGroup
	for i := range 16 {
		wg.Go(func() {
			size := 4 + 4*(i%2)
			split := i%4 >= 2
			c := testContext(size, split)
			u := c.Typecheck.IR.Types
			link := c.Runtime.Writer.Link
			typ := u.NewMap(u.Types[types.TSTRING], u.Types[types.TINT])
			sym := c.TypeLinksym(typ)
			if sym.TypeInfo().Type != typ || sym != c.TypeLinksym(typ) || sym != u.TypeSym(typ).Linksym() || len(c.signatslice) != 1 || len(c.signatset) != 1 {
				t.Error("type metadata identity or deduplication")
			}
			pos := link.PosTable.XPos(src.MakePos(src.NewFileBase("p.go", "/src/p.go"), 7, 3))
			ref := c.TypePtrAt(pos, typ)
			if ref.X.(*ir.LinksymOffsetExpr).Linksym != sym || ref.Type().Elem() != u.Types[types.TUINT8] || ref.Pos() != pos || ref.Typecheck() != 1 {
				t.Error("native checked runtime type reference")
			}
			from := link.Lookup("p.F")
			c.MarkTypeUsedInInterface(typ, from)
			if len(from.R) != 1 || from.R[0].Type != objabi.R_USEIFACE || from.R[0].Sym != sym {
				t.Error("interface reachability relocation")
			}
			group := c.MapGroupType(typ)
			nfields := 2
			if split {
				nfields = 3
			}
			if group != c.MapGroupType(typ) || group.NumFields() != nfields || group.StructType().Map != typ || !group.Noalg() {
				t.Error("map group layout or identity")
			}
			mt, iter := c.MapType(), c.MapIterType()
			if mt != c.MapType() || iter != c.MapIterType() || iter.Field(3).Type.Elem() != mt || mt.Size() != int64(16+4*size) || iter.Size() != int64(32+8*size) {
				t.Error("native map cache or layout")
			}
			oldName := sym.Name
			oldGroupSize := group.Size()
			oldRef := fmt.Sprint(ref)
			other := testContext(12-size, !split)
			otherType := other.Typecheck.IR.Types.NewMap(other.Typecheck.IR.Types.Types[types.TSTRING], other.Typecheck.IR.Types.Types[types.TINT])
			otherSym := other.TypeLinksym(otherType)
			other.MapGroupType(otherType)
			other.MapType()
			other.MapIterType()
			if sym == otherSym || oldName != sym.Name || sym.Name != otherSym.Name || len(c.signatslice) != 1 || c.signatslice[0].t != typ || group.Size() != oldGroupSize || fmt.Sprint(ref) != oldRef || other.MapType() == mt {
				t.Error("another compilation modified retained reflection state")
			}
			if c.TypeLinksym(typ) != sym || c.MapGroupType(typ) != group {
				t.Error("retained instance cannot resume")
			}
		})
	}
	wg.Wait()
}
func TestReflectionRejectsForeignAndInvalidTypes(t *testing.T) {
	c := testContext(8, false)
	other := testContext(4, true)
	u := c.Typecheck.IR.Types
	bad := other.Typecheck.IR.Types.Types[types.TINT]
	from := c.Runtime.Writer.Link.Lookup("p.F")
	recv := u.NewField(src.XPos{}, nil, u.Types[types.TINT])
	method := u.NewSignature(recv, nil, nil)
	shape := u.NewStruct(nil)
	shape.SetHasShape(true)
	for _, fn := range []func(){
		func() { NewContext(c.Typecheck, other.Runtime, c.StaticData) }, func() { NewContext(c.Typecheck, c.Runtime, other.StaticData) }, func() { c.TypeSym(bad) }, func() { c.TypeLinksym(bad) }, func() { c.TypeSymPrefix("eq", bad) }, func() { c.TypePtrAt(src.XPos{}, bad) }, func() { c.NeedRuntimeType(bad) }, func() { c.MapGroupType(other.Typecheck.IR.Types.NewMap(bad, bad)) },
		func() { c.TypeSym(nil) }, func() { c.TypeSym(u.UntypedInt) }, func() { c.TypeSym(method) }, func() { c.MarkTypeUsedInInterface(shape, from) },
	} {
		mustPanic(t, fn)
	}
	if len(c.signatslice) != 0 || len(c.signatset) != 0 || len(from.R) != 0 {
		t.Fatal("rejected call changed emission queue or symbol")
	}
	// Rejected calls must not poison the queue mutex.
	if c.TypeLinksym(u.Types[types.TINT]).TypeInfo().Type != u.Types[types.TINT] {
		t.Fatal("valid use after rejection")
	}
}
func TestNeedEmitUsesCompilationPackageAndTypeFlags(t *testing.T) {
	c := testContext(8, false)
	u := c.Typecheck.IR.Types
	graph := c.Typecheck.IR
	named := func(pkg *types.Pkg, name string) *types.Type {
		n := graph.NewDeclNameAt(src.XPos{}, ir.OTYPE, pkg.Lookup(name))
		typ := u.NewNamed(n)
		n.SetType(typ)
		typ.SetUnderlying(u.NewStruct(nil))
		return typ
	}
	foreign := named(u.NewPkg("example/dep", "dep"), "T")
	if c.NeedEmit(u.Types[types.TINT]) || c.NeedEmit(foreign) || !c.NeedEmit(named(u.LocalPkg, "T")) || !c.NeedEmit(u.NewStruct(nil)) {
		t.Fatal("package ownership emission policy")
	}
	foreign.SetIsFullyInstantiated(true)
	if !c.NeedEmit(foreign) {
		t.Fatal("instantiated type omitted")
	}
	foreign.SetIsFullyInstantiated(false)
	foreign.SetHasShape(true)
	if !c.NeedEmit(foreign) {
		t.Fatal("shape omitted")
	}
	c.Runtime.Writer.Link.Pkgpath = "runtime"
	if !c.NeedEmit(u.Types[types.TINT]) || !c.NeedEmit(u.NewSlice(u.Types[types.TINT])) {
		t.Fatal("runtime basic type ownership")
	}
}

func TestRuntimeReferenceHelpersRejectWrongOperations(t *testing.T) {
	c := testContext(8, false)
	graph := c.Typecheck.IR
	u := graph.Types
	value := graph.NewNameAt(src.XPos{}, u.LocalPkg.Lookup("v"), u.Types[types.TINT])
	cached := c.TypePtrAt(src.XPos{}, u.Types[types.TINT])
	n := graph.NewBinaryExpr(src.XPos{}, ir.OADD, value, value)
	n.RType = cached
	// An existing dictionary expression does not bypass native opcode assertions.
	mustPanic(t, func() { c.CopyElemRType(src.XPos{}, n) })
	n.SetOp(ir.OEQ)
	n.RType = nil
	mustPanic(t, func() { c.CompareRType(src.XPos{}, n) })
	call := graph.NewCallExpr(src.XPos{}, ir.OAPPEND, nil, []ir.Node{value})
	call.SetType(u.Types[types.TINT])
	mustPanic(t, func() { c.AppendElemRType(src.XPos{}, call) })
	if len(c.signatslice) != 1 || c.signatslice[0].t != u.Types[types.TINT] {
		t.Fatal("invalid helper registered a type")
	}
}

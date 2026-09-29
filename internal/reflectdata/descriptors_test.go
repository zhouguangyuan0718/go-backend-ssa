package reflectdata

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"go-backend-ssa/internal/cmd/obj"
	"go-backend-ssa/internal/cmd/objabi"
	"go-backend-ssa/internal/cmd/src"
	"go-backend-ssa/internal/go/abi"
	"go-backend-ssa/internal/ir"
	"go-backend-ssa/internal/types"
	"sync"
	"testing"
)

func descriptorContext(size int) *Context {
	c := algorithmContext(size, true)
	c.Runtime.Writer.Link.Pkgpath = c.Typecheck.IR.Types.LocalPkg.Path
	return c
}
func descriptorNamed(c *Context, name string, under *types.Type) *types.Type {
	g, u := c.Typecheck.IR, c.Typecheck.IR.Types
	n := g.NewDeclNameAt(src.XPos{}, ir.OTYPE, u.LocalPkg.Lookup(name))
	typ := u.NewNamed(n)
	n.SetType(typ)
	n.SetTypecheck(1)
	typ.SetUnderlying(under)
	return typ
}
func descriptorMethods(c *Context, names ...string) (*types.Type, *types.Type) {
	u := c.Typecheck.IR.Types
	typ := descriptorNamed(c, "Concrete", u.NewStruct([]*types.Field{u.NewField(src.XPos{}, u.LocalPkg.Lookup("Value"), u.Types[types.TSTRING])}))
	var methods, imethods []*types.Field
	for _, name := range names {
		sym := u.LocalPkg.Lookup(name)
		methods = append(methods, u.NewField(src.XPos{}, sym, u.NewSignature(u.NewField(src.XPos{}, nil, typ), nil, nil)))
		imethods = append(imethods, u.NewField(src.XPos{}, sym, u.NewSignature(u.NewField(src.XPos{}, nil, u.FakeRecvType()), nil, nil)))
	}
	typ.SetMethods(methods)
	iface := u.NewInterface(imethods)
	u.CalcSize(iface)
	return typ, iface
}
func symbolReloc(t *testing.T, s *obj.LSym, off int64) obj.Reloc {
	t.Helper()
	for _, r := range s.R {
		if int64(r.Off) == off {
			return r
		}
	}
	t.Fatalf("missing relocation at %d in %s", off, s.Name)
	return obj.Reloc{}
}
func TestNativeTypeDescriptors(t *testing.T) {
	for _, size := range []int{4, 8} {
		c := descriptorContext(size)
		u := c.Typecheck.IR.Types
		typ := descriptorNamed(c, "Record", u.NewStruct([]*types.Field{u.NewField(src.XPos{}, u.LocalPkg.Lookup("P"), u.NewPtr(u.Types[types.TINT])), u.NewField(src.XPos{}, u.LocalPkg.Lookup("S"), u.Types[types.TSTRING])}))
		c.NeedRuntimeType(typ)
		c.WriteRuntimeTypes()
		s := c.TypeLinksym(typ)
		if !s.OnList() || s.TypeInfo().Type != typ || s.Size <= c.Runtime.StructType.Size() || s.Align != int16(size) || len(c.signatslice) != 0 {
			t.Fatal("type descriptor registration")
		}
		if binary.LittleEndian.Uint32(s.P[c.Runtime.Type.OffsetOf("Hash"):]) != u.TypeHash(typ) {
			t.Fatal("native type hash")
		}
		flags := abi.TFlag(s.P[c.Runtime.Type.OffsetOf("TFlag")])
		if flags&(abi.TFlagNamed|abi.TFlagUncommon) != (abi.TFlagNamed|abi.TFlagUncommon) || s.P[c.Runtime.Type.OffsetOf("Kind_")] != uint8(abi.Struct) {
			t.Fatal("native type flags", flags)
		}
		eq := symbolReloc(t, s, c.Runtime.Type.OffsetOf("Equal")).Sym
		if eq != c.geneq(typ) || len(eq.R) != 1 || eq.R[0].Sym.ABI() != obj.ABIInternal {
			t.Fatal("descriptor equality closure")
		}
		count := len(c.Runtime.Writer.Link.Data)
		c.WriteRuntimeTypes()
		if len(c.Runtime.Writer.Link.Data) != count {
			t.Fatal("queue not drained")
		}
		large := u.NewArray(u.NewPtr(u.Types[types.TINT]), 129)
		c.GCSym(large, true)
		ls := c.writeType(large)
		if abi.TFlag(ls.P[c.Runtime.Type.OffsetOf("TFlag")])&abi.TFlagGCMaskOnDemand == 0 {
			t.Fatal("missing on-demand mask")
		}
		mask := symbolReloc(t, ls, c.Runtime.Type.OffsetOf("GCData")).Sym
		if mask.Type != objabi.SNOPTRBSS || mask.Size != int64(size) {
			t.Fatal("on-demand cell")
		}
		// The deferred backend queue also retains its original complete-mask path.
		c.WriteGCSymbols()
		full, _ := c.GCSym(large, false)
		if !full.OnList() || full == mask {
			t.Fatal("ordinary deferred mask")
		}
	}
}
func TestNativeITabsAndInterfaceConversions(t *testing.T) {
	c := descriptorContext(8)
	g, u := c.Typecheck.IR, c.Typecheck.IR.Types
	typ, iface := descriptorMethods(c, "M", "N")
	pos := c.Runtime.Writer.Link.PosTable.XPos(src.MakePos(src.NewFileBase("p.go", "p.go"), 9, 2))
	fn := g.NewFunc(pos, pos, u.LocalPkg.Lookup("Caller"), u.NewSignature(nil, nil, nil))
	u.Pos, g.CurFunc = pos, fn
	tab := c.ITabLsym(typ, iface)
	if u.Pos != pos || g.CurFunc != fn || tab.ItabInfo().Type != typ || c.ITabLsym(typ, iface) != tab || tab.Size != c.Runtime.ITab.Size()+8 {
		t.Fatal("itab identity/metadata/scope")
	}
	start := c.Runtime.ITab.OffsetOf("Fun")
	for i := int64(0); i < 2; i++ {
		r := symbolReloc(t, tab, start+8*i)
		if r.Type != objabi.R_WEAKADDR || r.Sym.ABI() != obj.ABIInternal {
			t.Fatal("itab weak function pointer")
		}
	}
	input := g.NewNameAt(pos, u.LocalPkg.Lookup("x"), typ)
	conv := g.NewConvExpr(pos, ir.OCONVIFACE, iface, input)
	got := c.ConvIfaceTypeWord(pos, conv).(*ir.AddrExpr).X.(*ir.LinksymOffsetExpr).Linksym
	if got != tab {
		t.Fatal("conversion uses different itab")
	}
	cached := c.TypePtrAt(pos, typ)
	conv.TypeWord = cached
	if c.ConvIfaceTypeWord(pos, conv) != cached {
		t.Fatal("dictionary type word replaced")
	}
	missing := descriptorNamed(c, "Missing", u.NewStruct(nil))
	dummy := c.ITabLsym(missing, iface)
	if dummy.Size != c.Runtime.ITab.Size() || binary.LittleEndian.Uint64(dummy.P[start:]) != 0 || len(dummy.R) != 2 {
		t.Fatal("native dummy itab")
	}
	other := descriptorContext(4)
	mustPanic(t, func() { c.ITabLsym(other.Typecheck.IR.Types.Types[types.TINT], iface) })
	// Invalid input is tested in a fresh instance: native failed symbol creation
	// is not a transaction and may leave partial declarations in that instance.
	bad := descriptorContext(8)
	_, bi := descriptorMethods(bad, "M")
	mustPanic(t, func() { bad.ITabAddrAt(src.XPos{}, bad.Typecheck.IR.Types.Types[types.TINT], bi) })
}
func TestIndependentDescriptorsAndNameCounters(t *testing.T) {
	var wg sync.WaitGroup
	for i := range 12 {
		wg.Go(func() {
			c := descriptorContext(4 + 4*(i%2))
			typ, iface := descriptorMethods(c, "hidden")
			tab := c.ITabLsym(typ, iface)
			data := bytes.Clone(tab.P)
			record := c.TypeLinksym(typ)
			recordBytes := bytes.Clone(record.P)
			functions := len(c.Typecheck.Target.Funcs)
			sym := c.dname("hidden", "", c.Typecheck.IR.Types.LocalPkg, false, false)
			before := c.dnameCount
			other := descriptorContext(12 - c.ptrSize)
			ot, oi := descriptorMethods(other, "hidden")
			otherTab := other.ITabLsym(ot, oi)
			other.dname("hidden", "", other.Typecheck.IR.Types.LocalPkg, false, false)
			if tab == otherTab || !bytes.Equal(data, tab.P) || !bytes.Equal(recordBytes, record.P) || c.dnameCount != before || len(c.Typecheck.Target.Funcs) != functions || sym == other.Runtime.Writer.Link.Lookup(sym.Name) {
				t.Error("other compilation changed descriptors")
			}
			if !bytes.Equal(tab.P, c.ITabLsym(typ, iface).P) {
				t.Error("retained descriptor")
			}
		})
	}
	wg.Wait()
}
func TestNativeNameEncodingAndMapDescriptors(t *testing.T) {
	for _, split := range []bool{false, true} {
		c := descriptorContext(8)
		u := c.Typecheck.IR.Types
		u.Experiment.MapSplitGroup = split
		pkg := u.NewPkg("example/dep", "dep")
		s := c.dname("field", "json:\"field\"", pkg, false, true)
		if s.P[0] != 14 || s.P[1] != 5 || string(s.P[2:7]) != "field" || len(s.R) != 1 || s.R[0].Siz != 4 || s.R[0].Sym != pkg.Pathsym {
			t.Fatal("native name flags/package relocation")
		}
		key := u.NewArray(u.Types[types.TUINT8], 129)
		elem := u.NewArray(u.Types[types.TSTRING], 9)
		m := u.NewMap(key, elem)
		sym := c.writeType(m)
		flags := binary.LittleEndian.Uint32(sym.P[c.Runtime.MapType.OffsetOf("Flags"):])
		if flags&(abi.MapIndirectKey|abi.MapIndirectElem) != (abi.MapIndirectKey | abi.MapIndirectElem) {
			t.Fatal("indirect map metadata", flags)
		}
		group := c.MapGroupType(m)
		if binary.LittleEndian.Uint64(sym.P[c.Runtime.MapType.OffsetOf("GroupSize"):]) != uint64(group.Size()) {
			t.Fatal("map group size")
		}
		if symbolReloc(t, sym, c.Runtime.MapType.OffsetOf("Hasher")).Sym != c.genhash(key) {
			t.Fatal("map hasher")
		}
		if fmt.Sprint(c.Typecheck.IR.Diagnostics.Diagnostics()) != "[]" {
			t.Fatal("descriptor diagnostics")
		}
	}
}

package noder

import (
	"bytes"
	"fmt"
	"go-backend-ssa/diagnostic"
	"go-backend-ssa/internal/cmd/src"
	"go-backend-ssa/internal/go/pkgbits"
	"go-backend-ssa/internal/ir"
	"go-backend-ssa/internal/types"
	"strings"
	"sync"
	"testing"
)

func shapeNamed(c *Context, name string, under *types.Type) *types.Type {
	n := c.Typecheck.IR.NewDeclNameAt(src.XPos{}, ir.OTYPE, c.Types.LocalPkg.Lookup(name))
	typ := c.Types.NewNamed(n)
	n.SetType(typ)
	n.SetTypecheck(1)
	if under != nil {
		typ.SetUnderlying(under)
	}
	return typ
}
func TestNativeShapesAndSharedCompilerState(t *testing.T) {
	c := writerContext(t)
	u, g := c.Types, c.Typecheck.IR
	if c.Base != g.Base || c.Flag != &g.Base.Flag || c.Debug != &g.Debug || c.Diagnostics != &g.Diagnostics || u.PosTable != &c.Ctxt.PosTable {
		t.Fatal("noder does not share compiler state")
	}
	named := shapeNamed(c, "Int", u.Types[types.TINT])
	a, b := c.Shapify(named, false), c.Shapify(u.Types[types.TINT], false)
	if a != b || !a.IsShape() || !a.HasShape() || a.Sym().Pkg != u.ShapePkg || a.Underlying() != u.Types[types.TINT] {
		t.Fatal("native shape canonicalization")
	}
	p, q := u.NewPtr(u.Types[types.TINT]), u.NewPtr(u.Types[types.TSTRING])
	if c.Shapify(p, true) != c.Shapify(q, true) || c.Shapify(p, false) == c.Shapify(q, false) {
		t.Fatal("basic constraint pointer collapse")
	}
	nonheap := shapeNamed(c, "Nonheap", u.NewStruct(nil))
	nonheap.SetNotInHeap(true)
	if c.Shapify(u.NewPtr(nonheap), true) == c.Shapify(p, true) {
		t.Fatal("not-in-heap pointer collapsed")
	}
	recursive := shapeNamed(c, "Recursive", nil)
	recursive.SetIsFullyInstantiated(true)
	recursive.SetHasShape(true)
	g.Debug.Shapify = 1
	if c.Shapify(recursive, false) != recursive || !strings.Contains(fmt.Sprint(g.Diagnostics.Diagnostics()), "skipping shaping") {
		t.Fatal("recursive shape or shared warning")
	}
	iface := u.NewInterface(nil)
	iface.SetIsFullyInstantiated(true)
	iface.SetHasShape(true)
	if c.Shapify(iface, false) != iface {
		t.Fatal("recursive instantiated interface replaced")
	}
	g.Debug.MaxShapeLen = 8
	long := u.NewStruct([]*types.Field{u.NewField(src.XPos{}, u.LocalPkg.Lookup(strings.Repeat("Field", 30)), u.Types[types.TINT])})
	hashed := c.Shapify(long, false)
	if len(hashed.Sym().Name) != 64 || hashed != c.Shapify(long, false) {
		t.Fatal("native long-name hashing")
	}
	other := writerContext(t)
	mustDictionaryPanic(t, func() { c.Shapify(other.Types.Types[types.TINT], false) })
	mustDictionaryPanic(t, func() { c.Shapify(shapeNamed(c, "Unresolved", nil), false) })
	// Writing again must not clear shared diagnostics or replace retained shapes.
	before := len(g.Diagnostics.Diagnostics())
	pkg, info, files := checked(t, c.Checker, "package p;var X int")
	if _, err := c.WritePackage(pkg, info, files, nil); err != nil {
		t.Fatal(err)
	}
	if len(g.Diagnostics.Diagnostics()) != before || c.Shapify(named, false) != a {
		t.Fatal("writer reset live compilation state")
	}
	var trace bytes.Buffer
	g.TraceWriter = &trace
	c.tracePrintf("shared trace")
	if trace.String() != "shared trace" {
		t.Fatal("shared trace")
	}
}
func TestNativeDictionaryFromWriter(t *testing.T) {
	c := writerContext(t)
	u := c.Types
	source := `package p
 type Box[T any] struct{V T}
 func Id[T any](v T)T{return v}
 func Use[T interface{M() int}](v T) any { _=v.M(); return v }
 func Call[T any](v T)any{_=Id(v);_=map[int]T{};return v}
 var X Box[int]
 `
	pkg, info, files := checked(t, c.Checker, source)
	data, err := c.WritePackage(pkg, info, files, nil)
	if err != nil {
		t.Fatal(err)
	}
	pr := c.newPkgReader(pkgbits.NewPkgDecoder(pkg.Path(), data))
	methodArg := shapeNamed(c, "MethodArg", u.Types[types.TINT])
	methodArg.SetMethods([]*types.Field{u.NewField(src.XPos{}, u.LocalPkg.Lookup("M"), u.NewSignature(u.NewField(src.XPos{}, nil, methodArg), nil, []*types.Field{u.NewField(src.XPos{}, nil, u.Types[types.TINT])}))})
	found := map[string]bool{}
	for i := range pr.NumElems(pkgbits.SectionName) {
		idx := index(i)
		r := pr.newReader(pkgbits.SectionName, idx, pkgbits.SyncObject1)
		_, sym := r.qualifiedIdent()
		tag := pkgbits.CodeObj(r.Code(pkgbits.SyncCodeObj))
		if tag == pkgbits.ObjStub || sym == nil || sym.Pkg != u.LocalPkg || !map[string]bool{"Box": true, "Id": true, "Use": true, "Call": true}[sym.Name] {
			continue
		}
		found[sym.Name] = true
		for _, shaped := range []bool{false, true} {
			argument := u.Types[types.TINT]
			if sym.Name == "Use" {
				argument = methodArg
			}
			args := []*types.Type{argument}
			dict, err := pr.objDictIdx(sym, idx, nil, args, shaped)
			if err != nil {
				t.Fatal(err)
			}
			if len(dict.targs) != 1 || dict.shaped != shaped || dict.targs[0].HasShape() != shaped || args[0] != argument || !strings.HasPrefix(dict.baseSym.Name, sym.Name+"[") {
				t.Fatal("native dictionary identity or input mutation")
			}
			if len(dict.derived) != len(dict.derivedTypes) || dict.varType(c).NumElem() != dict.numWords() {
				t.Fatal("dictionary storage")
			}
			if sym.Name == "Use" && len(dict.typeParamMethodExprs) == 0 {
				t.Fatal("missing type parameter method")
			}
			if sym.Name == "Call" && len(dict.subdicts) == 0 {
				t.Fatal("missing subdictionary")
			}
			again, err := pr.objDictIdx(sym, idx, nil, args, shaped)
			if err != nil || again == dict || again.baseSym != dict.baseSym || again.targs[0] != dict.targs[0] {
				t.Fatal("fresh dictionary/shared symbols")
			}
		}
		if _, err := pr.objDictIdx(sym, idx, nil, nil, false); err == nil {
			t.Fatal("wrong arity accepted")
		}
	}
	if len(found) != 4 {
		t.Fatal("missing generic declarations", found)
	}
}
func TestNativeDictionaryMangleAndSignature(t *testing.T) {
	c := writerContext(t)
	u, g := c.Types, c.Typecheck.IR
	cases := []struct {
		name                 string
		implicits, receivers int
		want                 string
	}{
		{"F", 0, 0, "F[int,string]"}, {"Local·7", 1, 0, "Local[int;string]·7"}, {"(*Box).M", 0, 1, "(*Box[int]).M[string]"},
	}
	for _, entry := range cases {
		dict := &readerDict{targs: []*types.Type{u.Types[types.TINT], u.Types[types.TSTRING]}, implicits: entry.implicits, receivers: entry.receivers}
		if got := dict.mangle(c, u.LocalPkg.Lookup(entry.name)).Name; got != entry.want {
			t.Fatal(got, entry.want)
		}
	}
	dict := &readerDict{typeParamMethodExprs: make([]readerMethodExprInfo, 2), subdicts: make([]objInfo, 3), rtypes: make([]typeInfo, 4), itabs: make([]itabInfo, 5)}
	if dict.subdictsOffset() != 2 || dict.rtypesOffset() != 5 || dict.itabsOffset() != 9 || dict.numWords() != 14 {
		t.Fatal("native dictionary sections")
	}
	recv := u.NewField(src.XPos{}, u.LocalPkg.Lookup("recv"), u.Types[types.TINT])
	param := u.NewField(src.XPos{}, u.LocalPkg.Lookup("args"), u.NewSlice(u.Types[types.TSTRING]))
	param.SetIsDDD(true)
	sig := u.NewSignature(recv, []*types.Field{param}, []*types.Field{u.NewField(src.XPos{}, nil, u.Types[types.TINT])})
	fn := g.NewFunc(src.XPos{}, src.XPos{}, u.LocalPkg.Lookup("F"), sig)
	shaped := c.shapeSig(fn, dict)
	if !shaped.HasShape() || shaped.NumParams() != 2 || !shaped.IsVariadic() || shaped.Param(0).Sym.Name != dictParamName || shaped.Param(0).Type.Elem().NumElem() != 14 || shaped.Recv() == recv || sig.NumParams() != 1 || sig.HasShape() {
		t.Fatal("native shaped function signature")
	}
}
func TestConcurrentDictionariesRetainGraph(t *testing.T) {
	var wg sync.WaitGroup
	for i := range 12 {
		wg.Go(func() {
			c := writerContext(t)
			u := c.Types
			typ := shapeNamed(c, fmt.Sprint("T", i), u.Types[types.TINT])
			shape := c.Shapify(typ, false)
			dict := &readerDict{targs: []*types.Type{shape}}
			symbol := dict.mangle(c, u.LocalPkg.Lookup("F"))
			old := symbol.Name
			other := writerContext(t)
			otherShape := other.Shapify(other.Types.Types[types.TINT], false)
			if shape == otherShape || shape.Sym().Def.Type() != shape || symbol.Name != old || dict.targs[0] != shape || c.Shapify(typ, false) != shape {
				t.Error("another compilation modified shape graph")
			}
			c.Diagnostics.Add(diagnostic.Warning, "", src.Pos{}, old)
			if other.Diagnostics.HasErrors() || len(other.Diagnostics.Diagnostics()) != 0 {
				t.Error("shared diagnostics across compilers")
			}
		})
	}
	wg.Wait()
}
func mustDictionaryPanic(t *testing.T, fn func()) {
	t.Helper()
	defer func() {
		if recover() == nil {
			t.Error("expected panic")
		}
	}()
	fn()
}

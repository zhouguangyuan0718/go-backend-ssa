package consumer

import (
	"encoding/binary"
	"fmt"
	"go-backend-ssa/diagnostic"
	"go-backend-ssa/gcbridge"
	"go-backend-ssa/ir"
	"go-backend-ssa/noder"
	"go-backend-ssa/objw"
	"go-backend-ssa/position"
	"go-backend-ssa/rttype"
	"go-backend-ssa/syntax"
	"go-backend-ssa/target"
	"go-backend-ssa/typecheck"
	"go-backend-ssa/types"
	"go-backend-ssa/types2"
	"go/constant"
	"strings"
	"testing"
)

func TestPublicObjects(t *testing.T) {
	layout, err := target.NewLayout("linux", "386")
	if err != nil || layout.PointerSize() != 4 {
		t.Fatal(layout, err)
	}
	u, err := types.NewUniverse(layout)
	if err != nil {
		t.Fatal(err)
	}
	scalar, err := u.Scalar(types.Int)
	if err != nil {
		t.Fatal(err)
	}
	var nativeType *gcbridge.Type = scalar
	pkg := u.NewPkg("example", "example")
	var nativeSymbol *gcbridge.Sym = pkg.Lookup("Value")
	pkg.Pathsym = &gcbridge.LinkSymbol{Name: "example"}
	if nativeType.Universe() != u || nativeSymbol.Universe() != u || pkg.Pathsym.Name != "example" {
		t.Fatal("type/symbol bridge changed object identity")
	}
	var positions position.Table
	p := position.Make(position.NewFile("main.go", "/main.go"), 7, 3)
	var native gcbridge.Pos = p
	var nativeTable *gcbridge.PosTable = &positions
	var compressed position.XPos = gcbridge.Encode(nativeTable, native)
	if gcbridge.Decode(nativeTable, compressed) != p {
		t.Fatal("different object model")
	}
	var diagnostics diagnostic.Collector
	diagnostics.Add(diagnostic.Warning, "probe", p, "example")
	if diagnostics.Diagnostics()[0].String() != "main.go:7:3: example" {
		t.Fatal(diagnostics.Diagnostics())
	}
}

// Object remains implementable outside the compiler's internal package tree.
type binding struct {
	sym *types.Sym
	typ *types.Type
}

func (b binding) Pos() position.XPos { return position.XPos{} }
func (b binding) Sym() *types.Sym    { return b.sym }
func (b binding) Type() *types.Type  { return b.typ }

var _ types.Object = binding{}
var _ interface{ CanBeAnSSAAux() } = (*types.Type)(nil)

func TestBackendSSATypeInputs(t *testing.T) {
	layout, err := target.NewLayout("linux", "amd64")
	if err != nil {
		t.Fatal(err)
	}
	u, err := types.NewUniverse(layout)
	if err != nil {
		t.Fatal(err)
	}
	scalar, err := u.Scalar(types.Int)
	if err != nil {
		t.Fatal(err)
	}
	var tuple *gcbridge.Type = u.NewTuple(scalar, u.TypeMem)
	results := u.NewResults([]*types.Type{u.TypeMem})
	if !tuple.IsTuple() || !tuple.IsMemory() || results != u.TypeResultMem || !results.IsResults() {
		t.Fatal("public backend cannot preserve SSA type identity")
	}
	first, second, err := tuple.TupleTypes()
	if err != nil || first != scalar || second != u.TypeMem {
		t.Fatal("tuple children not accessible")
	}
}

func TestPublicCompositeTypes(t *testing.T) {
	layout, _ := target.NewLayout("linux", "386")
	u, _ := types.NewUniverse(layout)
	value, _ := u.Scalar(types.Int)
	ptr := u.NewPtr(value)
	slice := u.NewSlice(ptr)
	array := u.NewArray(slice, 4)
	channel := u.NewChan(value, types.SendRecv)
	mapping := u.NewMap(value, ptr)
	if array.NumElem() != 4 || array.Elem().Elem() != ptr || ptr.Elem() != value || mapping.Key() != value || channel.ChanDir() != types.SendRecv {
		t.Fatal("cannot inspect composite graph through public package")
	}
}

func TestPublicConstantTypes(t *testing.T) {
	layout, _ := target.NewLayout("linux", "amd64")
	u, _ := types.NewUniverse(layout)
	integer := u.UntypedTypes[constant.Int]
	if integer != u.UntypedInt || !integer.IsInteger() || !integer.IsUntyped() || u.DefaultKinds[constant.Int] != types.Int {
		t.Fatal("public constant mapping unavailable")
	}
	if (*types.Type)(nil).IsUntyped() {
		t.Fatal("nil behavior changed")
	}
}

func TestPublicSignatureGraph(t *testing.T) {
	layout, _ := target.NewLayout("linux", "amd64")
	u, _ := types.NewUniverse(layout)
	value, _ := u.Scalar(types.Int)
	field := u.NewField(position.XPos{}, nil, value)
	record := u.NewStruct([]*types.Field{field})
	signature := u.NewSignature(nil, []*types.Field{field}, []*types.Field{field})
	var native *gcbridge.Field = signature.Result(0)
	var nativeStruct *gcbridge.Struct = record.StructType()
	if native != field || nativeStruct != record.StructType() || signature.NumParams() != 1 || signature.ResultsTuple().FieldType(0) != value {
		t.Fatal("public signature graph lost native identity")
	}
	result := u.NewResults([]*types.Type{value, u.TypeMem})
	if result.NumFields() != 2 || result.FieldType(1) != u.TypeMem {
		t.Fatal("backend cannot inspect call results")
	}
}

func TestNativeLayoutFromPublicModule(t *testing.T) {
	layout, _ := target.NewLayout("linux", "386")
	u, err := types.NewUniverse(layout)
	if err != nil {
		t.Fatal(err)
	}
	u.InitTypes(func(sym *types.Sym, typ *types.Type) types.Object { return binding{sym, typ} })
	value, err := u.Scalar(types.Int)
	if err != nil {
		t.Fatal(err)
	}
	record := u.NewStruct([]*types.Field{u.NewField(position.XPos{}, nil, u.NewPtr(value)), u.NewField(position.XPos{}, nil, value)})
	var native *gcbridge.Type = record
	if native.Size() != 8 || native.Alignment() != 4 || u.PtrDataSize(native) != 4 || !u.IsComparable(native) || u.AlgType(native) != types.AMEM {
		t.Fatal("native layout API", native)
	}
	if !u.Identical(u.ByteType, u.Types[types.Uint8]) || u.Diagnostics.HasErrors() {
		t.Fatal("builtin binding")
	}
}

func TestNativeSyntaxFromPublicModule(t *testing.T) {
	f, err := syntax.Parse(syntax.NewFileBase("sum.go"), strings.NewReader("package p; func Sum(x,y int)int{return x+y}"), nil, nil, syntax.CheckBranches)
	if err != nil {
		t.Fatal(err)
	}
	var native *gcbridge.SyntaxFile = f
	var n gcbridge.SyntaxNode = f.DeclList[0]
	fn := n.(*syntax.FuncDecl)
	ret := fn.Body.List[0].(*syntax.ReturnStmt)
	if native != f || fn.Name.Value != "Sum" || ret.Results.(*syntax.Operation).Op != syntax.Add {
		t.Fatal("syntax bridge identity")
	}
}

func TestNativeCheckerFromPublicModule(t *testing.T) {
	file, err := syntax.Parse(syntax.NewFileBase("id.go"), strings.NewReader("package p;func Id[T any](x T)T{return x};var X=Id(1);func Each(y func(int)bool){y(1)};func Sum()(n int){for x:=range Each{n+=x};return}"), nil, nil, syntax.CheckBranches)
	if err != nil {
		t.Fatal(err)
	}
	state := types2.NewContext()
	var native *gcbridge.CheckerContext = state
	conf := types2.Config{Context: state, GoVersion: "go1.27", Sizes: state.SizesFor("gc", "amd64")}
	var options *gcbridge.CheckerConfig = &conf
	info := &types2.Info{StoreTypesInSyntax: true, Defs: map[*syntax.Name]types2.Object{}, Uses: map[*syntax.Name]types2.Object{}, Instances: map[*syntax.Name]types2.Instance{}, Selections: map[*syntax.SelectorExpr]*types2.Selection{}, Implicits: map[syntax.Node]types2.Object{}, Scopes: map[syntax.Node]*types2.Scope{}, FileVersions: map[*syntax.PosBase]string{}}
	p, err := options.Check("example/p", []*syntax.File{file}, info)
	if err != nil {
		t.Fatal(err)
	}
	var pkg *gcbridge.CheckedPackage = p
	var typ gcbridge.SourceType = pkg.Scope().Lookup("X").Type()
	if native != state || typ != state.Typ[types2.Int] {
		t.Fatal("checker bridge identity")
	}
	layout, _ := target.NewLayout("linux", "amd64")
	universe, err := types.NewUniverse(layout)
	if err != nil {
		t.Fatal(err)
	}
	link := &gcbridge.Link{Pathname: "/src", Arch: &gcbridge.LinkArch{Arch: &gcbridge.Arch{Name: "amd64", PtrSize: 8, RegSize: 8}}}
	writer, err := noder.NewContext(state, universe, link)
	if err != nil {
		t.Fatal(err)
	}
	closures, err := writer.RewriteRangeFuncs(p, info, []*syntax.File{file})
	if err != nil || len(closures) != 1 {
		t.Fatalf("native rangefunc preparation: %v", err)
	}
	data, err := writer.WritePackage(p, info, []*syntax.File{file}, closures)
	if err != nil || data == "" || p.Scope().Lookup("X").Type() != typ {
		t.Fatalf("native writer/retained graph: %v", err)
	}
	if p.Context() != state || state.PackageScope(nil) != state.Universe {
		t.Fatal("explicit package scope ownership")
	}
}

func TestNativeObjectGraph(t *testing.T) {
	// The host and migrated packages share positions, symbols and callbacks.
	link := &gcbridge.Link{Pkgpath: "example/native", Arch: &gcbridge.LinkArch{Arch: &gcbridge.Arch{Name: "test", PtrSize: 8, RegSize: 8}}}
	pos := link.PosTable.XPos(position.Make(position.NewFile("native.go", "/src/native.go"), 7, 2))
	symbol := &gcbridge.LinkSymbol{Name: "example/native.F"}
	prog := &gcbridge.Prog{Ctxt: link, Pos: pos, From: gcbridge.Addr{Sym: symbol}}
	link.Text = []*gcbridge.LinkSymbol{symbol}
	called := false
	link.Arch.Assemble = func(got *gcbridge.Link, sym *gcbridge.LinkSymbol, newProg gcbridge.ProgAlloc) {
		if got != link || sym != symbol || newProg() != prog {
			t.Error("native callback identity")
		}
		called = true
	}
	link.Arch.Assemble(link, symbol, func() *gcbridge.Prog { return prog })
	if !called || link.PosTable.Pos(prog.Pos).Line() != 7 || prog.From.Sym != link.Text[0] {
		t.Fatal("native graph identity")
	}
	// A second instance cannot mutate the first position table or symbol list.
	other := &gcbridge.Link{Pkgpath: "example/native"}
	other.PosTable.XPos(position.Make(position.NewFile("other.go", "/src/other.go"), 20, 1))
	other.Text = append(other.Text, &gcbridge.LinkSymbol{Name: symbol.Name})
	if link.PosTable.Pos(prog.Pos).Base().Filename() != "native.go" || other.Text[0] == symbol {
		t.Fatal("instance coupling")
	}
}

func TestSyntaxToNativePosition(t *testing.T) {
	link := &gcbridge.Link{Pathname: "/work/src"}
	m := noder.NewPosMap(link, "/work=>project", "")
	p := syntax.MakePos(syntax.NewFileBase("file.go"), 9, 3)
	x := m.XPos(p)
	if got := link.PosTable.Pos(x); got.Line() != 9 || got.Base().AbsFilename() != "project/src/file.go" {
		t.Fatal(got)
	}
}

func TestCompilerLinkSymbols(t *testing.T) {
	layout, _ := target.NewLayout("linux", "amd64")
	u, err := types.NewUniverse(layout)
	if err != nil {
		t.Fatal(err)
	}
	link := &gcbridge.Link{}
	u.BindLink(link)
	name := u.NewPkg("example/p", "p").Lookup("F")
	name.SetFunc(true)
	var symbol *gcbridge.LinkSymbol = name.Linksym()
	if symbol != link.LookupABI("example/p.F", gcbridge.ABIInternal) || symbol.Pkg != "example/p" {
		t.Fatal("native symbol namespace")
	}
	pos := link.PosTable.XPos(position.Make(position.NewFile("caller.go", "/src/caller.go"), 10, 2))
	i := link.InlTree.Add(-1, pos, symbol, "F")
	callee := position.NewInlining(position.NewFile("callee.go", "/src/callee.go"), i)
	inlined := link.PosTable.XPos(position.Make(callee, 20, 3))
	if link.OutermostPos(inlined).Line() != 10 || link.InnermostPos(inlined).Line() != 20 {
		t.Fatal("native inline position API")
	}
}

func TestNativeIRPublicGraph(t *testing.T) {
	layout, _ := target.NewLayout("linux", "amd64")
	link := &gcbridge.Link{}
	u, err := types.NewUniverseWithPositions(layout, &link.PosTable)
	if err != nil {
		t.Fatal(err)
	}
	c := ir.NewContext(u, link)
	tc := typecheck.NewContext(c)
	tc.InitUniverse()
	tc.InitRuntime()
	alloc := tc.LookupRuntime("newobject", u.Types[types.Int])
	if alloc.Type().Result(0).Type.Elem() != u.Types[types.Int] || alloc.Linksym() != tc.LookupRuntimeFunc("newobject") {
		t.Fatal("native runtime substitution or ABI binding")
	}

	if c.BlankNode != u.BlankSym.Def || u.BuiltinPkg.Lookup("append").Def.(*ir.Name).BuiltinOp != ir.OAPPEND {
		t.Fatal("native universe not installed")
	}

	typ := u.Types[types.Int]
	if _, ok := typ.Obj().(*ir.Name); !ok {
		t.Fatal("builtin type is not bound to native ir.Name")
	}
	value := c.NewBasicLit(position.XPos{}, typ, constant.MakeInt64(3))
	expr := c.NewBinaryExpr(position.XPos{}, ir.OADD, value, value)
	var node ir.Node = expr
	dup := ir.DeepCopy(position.XPos{}, node).(*ir.BinaryExpr)
	if dup == expr || dup.X != value || dup.Y != value || fmt.Sprint(dup) != "3 + 3" {
		t.Fatal("native copy or interface differs")
	}
	checked := tc.Expr(dup)
	if checked.Type() != typ || checked.Typecheck() != 1 {
		t.Fatal("public native expression checking")
	}
	if typecheck.RangeExprType(u.NewPtr(u.NewArray(typ, 2))).Kind() != types.Array {
		t.Fatal("public range operand type")
	}
	count := 0
	ir.Visit(dup, func(ir.Node) { count++ })
	if count != 3 {
		t.Fatal(count)
	}
}

func TestNativeObjectWriterPublicAPI(t *testing.T) {
	layout, _ := target.NewLayout("linux", "amd64")
	link := &gcbridge.Link{Arch: &gcbridge.LinkArch{Arch: &gcbridge.Arch{Name: "amd64", PtrSize: 8, RegSize: 8, ByteOrder: binary.LittleEndian}}}
	u, err := types.NewUniverseWithPositions(layout, &link.PosTable)
	if err != nil {
		t.Fatal(err)
	}
	c := objw.NewContext(u, link, "linux")
	data, targetSym := link.Lookup("p.dictionary"), link.Lookup("p.type")
	c.Uintptr(data, 0, 42)
	end := c.SymPtr(data, 8, targetSym, 0)
	c.Global(data, int32(end), objw.RODATA|objw.DUPOK)
	if binary.LittleEndian.Uint64(data.P) != 42 || len(data.R) != 1 || data.R[0].Sym != targetSym || data.R[0].Siz != 8 || data.Type.String() != "SRODATA" || !data.DuplicateOK() || link.Data[0] != data {
		t.Fatal("native object writer identity/bytes/relocation")
	}
}

func TestNativeRuntimeLayoutsPublicAPI(t *testing.T) {
	layout, _ := target.NewLayout("linux", "386")
	link := &gcbridge.Link{Arch: &gcbridge.LinkArch{Arch: &gcbridge.Arch{Name: "386", PtrSize: 4, RegSize: 4, ByteOrder: binary.LittleEndian}}}
	u, err := types.NewUniverseWithPositions(layout, &link.PosTable)
	if err != nil {
		t.Fatal(err)
	}
	typecheck.NewContext(ir.NewContext(u, link)).InitUniverse()
	c := rttype.NewContext(objw.NewContext(u, link, "linux"))
	c.Init()
	s, fn := link.Lookup("p.itab"), link.Lookup("p.method")
	var cursor rttype.Cursor = c.NewCursor(s, 0, c.ITab)
	cursor.Field("Hash").WriteUint32(123)
	methods, delta := cursor.Field("Fun").ModifyArray(2)
	methods.Elem(1).WritePtrWeak(fn)
	if c.ITab.Size() != 16 || delta != 4 || c.Type.Universe() != u || len(s.R) != 1 || s.R[0].Sym != fn || s.R[0].Off != 16 || s.R[0].Siz != 4 {
		t.Fatal("public runtime layout/relocation")
	}
}

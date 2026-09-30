package consumer

import (
	"encoding/binary"
	"fmt"
	"go-backend-ssa/abi"
	"go-backend-ssa/bitvec"
	"go-backend-ssa/compare"
	"go-backend-ssa/deadlocals"
	"go-backend-ssa/diagnostic"
	"go-backend-ssa/escape"
	"go-backend-ssa/gcbridge"
	"go-backend-ssa/inline"
	"go-backend-ssa/inline/inlheur"
	"go-backend-ssa/ir"
	"go-backend-ssa/logopt"
	"go-backend-ssa/loopvar"
	"go-backend-ssa/noder"
	"go-backend-ssa/objw"
	"go-backend-ssa/pgo"
	"go-backend-ssa/pgoir"
	"go-backend-ssa/pkginit"
	"go-backend-ssa/position"
	"go-backend-ssa/reflectdata"
	"go-backend-ssa/rttype"
	"go-backend-ssa/slice"
	"go-backend-ssa/ssa"
	"go-backend-ssa/ssagen"
	"go-backend-ssa/staticdata"
	"go-backend-ssa/staticinit"
	"go-backend-ssa/syntax"
	"go-backend-ssa/target"
	"go-backend-ssa/typebits"
	"go-backend-ssa/typecheck"
	"go-backend-ssa/types"
	"go-backend-ssa/types2"
	"go-backend-ssa/walk"
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
	file, err := syntax.Parse(syntax.NewFileBase("id.go"), strings.NewReader("package p;func Id[T any](x T)T{return x};var X=Id(1);func Each(y func(int)bool){y(1)};func Sum()(n int){for x:=range Each{n+=x};return};func UseId(x int)int{return Id(x)};func Twice(x int)int{return Id(Id(x))};type Number int;func(n Number)M(x int)int{return int(n)+x};func Method(n Number)func(int)int{return n.M};func Captures(xs []int)(out []*int){for _,v:=range xs{out=append(out,&v)};return};func Dead(p int){p=1};func RetPtr()*int{x:=7;return &x};func Grow(n int)[]int{var s []int;for i:=0;i<n;i++{s=append(s,i)};return s};func CompareStrings(a,b string)bool{return a==b}"), nil, nil, syntax.CheckBranches)
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
	link := &gcbridge.Link{Pathname: "/src", Pkgpath: "example/p", Arch: &gcbridge.LinkArch{Arch: &gcbridge.Arch{Name: "amd64", PtrSize: 8, RegSize: 8, ByteOrder: binary.LittleEndian}}}
	universe, err := types.NewUniverseWithPositions(layout, &link.PosTable)
	if err != nil {
		t.Fatal(err)
	}
	universe.LocalPkg = universe.NewPkg("example/p", "p")
	graph := ir.NewContext(universe, link)
	tc := typecheck.NewContext(graph)
	tc.InitUniverse()
	tc.InitRuntime()
	graph.Base.Flag.Lang = "go1.27"
	graph.Base.Flag.Complete = true
	graph.Debug.SyncFrames = -1
	graph.Debug.RangeFuncCheck = 1
	writer, err := noder.NewContext(state, tc)
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
	shape := writer.Shapify(universe.Types[types.Int], false)
	if shape.Universe() != universe || !shape.IsShape() || !shape.HasShape() || shape != writer.Shapify(universe.Types[types.Int], false) || writer.Flag != &graph.Base.Flag || writer.Debug != &graph.Debug || writer.Diagnostics != &graph.Diagnostics {
		t.Fatal("public reader shaping/shared graph")
	}
	if p.Context() != state || state.PackageScope(nil) != state.Universe {
		t.Fatal("explicit package scope ownership")
	}
	ow := objw.NewContext(universe, link, "linux")
	rt := rttype.NewContext(ow)
	rt.Init()
	writer.ReflectData = reflectdata.NewContext(tc, rt, staticdata.NewContext(graph, ow))
	writer.StaticInit = staticinit.NewContext(tc, writer.ReflectData)
	result := writer.ReadLocalPackage(data)
	sum, err := writer.LookupFunc("example/p.Sum")
	if err != nil {
		t.Fatal(err)
	}
	if result != tc.Target || len(sum.Body) == 0 || sum.Typecheck() != 1 || p.Scope().Lookup("X").Type() != typ {
		t.Fatal("public source to package IR or retained source types")
	}
	graph.Base.Flag.LowerL = 1
	writer.Inline.CanInlineFuncs(result.Funcs, nil)
	link.DwFixups = gcbridge.NewDwarfFixupTable(link)
	graph.InitLSym(sum, true)
	link.DwFixups.SetPrecursorFunc(sum.LSym, sum)
	if sum.LSym.Func() == nil || link.DwFixups.GetPrecursorFunc(sum.LSym) != sum {
		t.Fatal("native function symbol or DWARF precursor lost")
	}
	caller, err := writer.LookupFunc("example/p.UseId")
	if err != nil {
		t.Fatal(err)
	}
	var call *ir.CallExpr
	ir.VisitList(caller.Body, func(n ir.Node) {
		if v, ok := n.(*ir.CallExpr); ok {
			call = v
		}
	})
	if call == nil {
		t.Fatal("public source call missing")
	}
	graph.WithFunc(caller, func() {
		expanded := writer.Inline.TryInlineCall(caller, call, false, nil, false)
		if expanded == nil || expanded.Type() != call.Type() || len(expanded.Body) == 0 {
			t.Fatal("public native inline expansion failed")
		}
	})

	writer.Interleaved.DevirtualizeAndInlinePackage(result, nil)
	twice, err := writer.LookupFunc("example/p.Twice")
	if err != nil {
		t.Fatal(err)
	}
	expanded, calls, parens := 0, 0, 0
	ir.VisitList(twice.Body, func(n ir.Node) {
		switch n.Op() {
		case ir.OINLCALL:
			expanded++
		case ir.OCALLFUNC:
			calls++
		case ir.OPAREN:
			parens++
		}
	})
	if expanded < 2 || calls != 0 || parens != 0 || graph.CurFunc != nil {
		t.Fatal("public native package driver failed", expanded, calls, parens)
	}

	writer.MakeWrappers(result)
	var methodWrapper *ir.Func
	for _, fn := range result.Funcs {
		if fn.Sym().Name == "Number.M-fm" {
			methodWrapper = fn
		}
	}
	if methodWrapper == nil || !methodWrapper.Wrapper() || !methodWrapper.Needctxt() || len(methodWrapper.ClosureVars) != 1 || methodWrapper.Nname.Defn != methodWrapper {
		t.Fatal("public native wrapper generation failed")
	}

	capture := loopvar.NewContext(tc, writer.Inline.Log)
	var transformed []loopvar.VarAndLoop
	for _, fn := range result.Funcs {
		transformed = append(transformed, capture.ForCapture(fn)...)
	}
	if len(transformed) != 1 || transformed[0].Name.Curfn.Sym().Name != "Captures" || transformed[0].Loop.Op() != ir.ORANGE || graph.CurFunc != nil {
		t.Fatal("public loop capture rewrite failed", transformed)
	}

	initialization := pkginit.NewContext(writer)
	initialization.MakeTask()
	if len(initialization.InstrumentGlobalsMap) != 0 || graph.CurFunc != nil || pkginit.GetRedzoneSizeForGlobal(33) != 63 {
		t.Fatal("public initialization state or redzone calculation failed")
	}

	universe.Experiment.RegabiWrappers = true
	backend := ssagen.NewContext(tc)
	backend.ReadFile = func(string) ([]byte, error) { return []byte("ref example/p.Twice ABI0\n"), nil }
	symABIs := backend.NewSymABIs()
	symABIs.ReadSymABIs("input.symabis")
	symABIs.GenABIWrappers()
	wrappers := 0
	for _, fn := range result.Funcs {
		if fn.Sym().Name == "Twice" && fn.ABI == gcbridge.ABI0 && fn.ABIWrapper() {
			wrappers++
		}
	}
	if wrappers != 1 || graph.CurFunc != nil {
		t.Fatal("public ABI wrapper generation failed", wrappers)
	}

	deadlocals.NewContext(graph).Funcs(result.Funcs)
	dead, err := writer.LookupFunc("example/p.Dead")
	if err != nil {
		t.Fatal(err)
	}
	assignment := dead.Body[0].(*ir.AssignStmt)
	if assignment.X != graph.BlankNode || graph.Int64Val(assignment.Y) != 0 {
		t.Fatal("public deadlocals pass failed")
	}
	analysis := escape.NewContext(tc, writer.Inline.Log)
	analysis.Funcs(result.Funcs)
	ret, err := writer.LookupFunc("example/p.RetPtr")
	if err != nil {
		t.Fatal(err)
	}
	var heapLocal *ir.Name
	for _, n := range ret.Dcl {
		if n.Sym().Name == "x" {
			heapLocal = n
		}
	}
	if heapLocal == nil || heapLocal.Esc() != ir.EscHeap || heapLocal.Opt != nil {
		t.Fatal("public escape classification failed")
	}
	graph.Debug.VariableMakeThreshold = 64
	slice.NewContext(analysis, writer.ReflectData).Funcs(result.Funcs)
	grow, err := writer.LookupFunc("example/p.Grow")
	if err != nil {
		t.Fatal(err)
	}
	moves := 0
	ir.Visit(grow, func(n ir.Node) {
		if m, ok := n.(*ir.MoveToHeapExpr); ok {
			moves++
			if m.RType == nil || m.PreserveCapacity || !graph.OwnsNode(m) {
				t.Fatal("public slice move metadata")
			}
		}
	})
	if moves != 1 {
		t.Fatal("public slice optimization failed", moves)
	}
	cmpfn, err := writer.LookupFunc("example/p.CompareStrings")
	if err != nil {
		t.Fatal(err)
	}
	graph.WithFunc(cmpfn, func() {
		e := cmpfn.Body[0].(*ir.ReturnStmt).Results[0].(*ir.BinaryExpr)
		eq, call := compare.NewContext(tc).EqString(e.X, e.Y)
		if eq.Op() != ir.OEQ || call.Fun.(*ir.Name).Sym().Name != "memequal" || !graph.OwnsNode(eq) || !graph.OwnsNode(call) {
			t.Fatal("public comparison construction")
		}
	})

	abiLayout := abi.NewContext(graph)
	cfg := abiLayout.NewABIConfig(2, 2, 0, 1)
	assigned := cfg.ABIAnalyzeFuncType(cmpfn.Type())
	if assigned.Config() != cfg || len(assigned.InParams()) != 2 || len(assigned.OutParams()) != 1 || len(abiLayout.RegisterTypes(assigned.InParams())) != 2 {
		t.Fatal("public ABI layout", assigned)
	}
	lowering := walk.NewContext(writer, analysis)
	seen := map[*ir.Func]bool{}
	for _, root := range result.Funcs {
		todo := []*ir.Func{root}
		for len(todo) > 0 {
			fn := todo[len(todo)-1]
			todo = todo[:len(todo)-1]
			if seen[fn] {
				continue
			}
			seen[fn] = true
			if fn.LSym == nil {
				graph.InitLSym(fn, true)
			}
			graph.Types.CalcSize(fn.Type())
			lowering.Walk(fn)
			if graph.CurFunc != fn {
				t.Fatal("public Walk lost current function")
			}
			graph.CurFunc = nil
			todo = append(todo, fn.Closures...)
		}
	}
	if len(seen) == 0 {
		t.Fatal("public Walk did not process functions")
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

func TestNativeReflectionDataPublicAPI(t *testing.T) {
	layout, _ := target.NewLayout("linux", "386")
	link := &gcbridge.Link{Arch: &gcbridge.LinkArch{Arch: &gcbridge.Arch{Name: "386", PtrSize: 4, RegSize: 4, ByteOrder: binary.LittleEndian}}}
	u, err := types.NewUniverseWithPositions(layout, &link.PosTable)
	if err != nil {
		t.Fatal(err)
	}
	u.LocalPkg = u.NewPkg("example/p", "p")
	link.Pkgpath = u.LocalPkg.Path
	tc := typecheck.NewContext(ir.NewContext(u, link))
	tc.InitUniverse()
	rt := rttype.NewContext(objw.NewContext(u, link, "linux"))
	rt.Init()
	c := reflectdata.NewContext(tc, rt, staticdata.NewContext(tc.IR, rt.Writer))
	scheduler := staticinit.NewContext(tc, c)
	global := scheduler.StaticName(u.Types[types.Int])
	schedule := scheduler.NewSchedule()
	if !schedule.StaticAssign(global, 0, tc.IR.NewInt(position.XPos{}, 17), u.Types[types.Int]) || binary.LittleEndian.Uint32(global.Linksym().P) != 17 || len(schedule.Out) != 0 {
		t.Fatal("public static initialization")
	}

	typ := u.NewMap(u.Types[types.String], u.Types[types.Int])
	symbol := c.TypeLinksym(typ)
	ref := c.TypePtrAt(position.XPos{}, typ)
	var native *ir.AddrExpr = ref
	if native.X.(*ir.LinksymOffsetExpr).Linksym != symbol || native.Type().Elem() != u.Types[types.Uint8] || symbol.TypeInfo().Type != typ {
		t.Fatal("native reflection type identity")
	}
	from := link.Lookup("p.F")
	c.MarkTypeUsedInInterface(typ, from)
	if len(from.R) != 1 || from.R[0].Type.String() != "R_USEIFACE" || from.R[0].Sym != symbol {
		t.Fatal("interface-use marker")
	}
	group := c.MapGroupType(typ)
	if group != c.MapGroupType(typ) || group.StructType().Map != typ || c.MapType().Size() != 32 || c.MapIterType().Size() != 64 {
		t.Fatal("public native map layout")
	}
	var bitmap objw.BitVec = bitvec.New(1)
	typebits.Set(typ, 0, bitmap)
	if bitmap.Count() != 1 || !bitmap.Get(0) {
		t.Fatal("public native GC scan")
	}
	gc, ptrdata := c.GCSym(typ, false)
	if gc.OnList() || ptrdata != 4 {
		t.Fatal("GC symbols should be deferred")
	}
	c.WriteGCSymbols()
	if !gc.OnList() || gc.Size != 4 || gc.Align != 4 || binary.LittleEndian.Uint32(gc.P) != 1 {
		t.Fatal("public GC bitmap data")
	}
	zero := c.ZeroAddr(17).(*ir.AddrExpr)
	if c.ZeroSize != 17 || zero.X.(*ir.LinksymOffsetExpr).Linksym.Name != "go:map.zero" {
		t.Fatal("native zero extent")
	}

	tc.InitRuntime()
	array := u.NewArray(u.Types[types.String], 64)
	equal, needsLength := c.EqFor(array)
	fn := equal.(*ir.Name).Func
	if needsLength || fn == nil || !fn.Dupok() || !fn.NilCheckDisabled() || len(fn.Body) == 0 {
		t.Fatal("public native equality body")
	}
	loops := 0
	ir.VisitList(fn.Body, func(n ir.Node) {
		if n.Op() == ir.OFOR {
			loops++
		}
	})
	if loops == 0 || len(tc.Target.Funcs) == 0 {
		t.Fatal("generated equality was not registered")
	}
	before := len(tc.Target.Funcs)
	again, _ := c.EqFor(array)
	if again != equal || len(tc.Target.Funcs) != before {
		t.Fatal("public equality cache identity")
	}
	memory, length := c.EqFor(u.NewArray(u.Types[types.Int], 3))
	if !length || memory != tc.LookupRuntime("memequal") {
		t.Fatal("public runtime equality selection")
	}

	pos := position.XPos{}
	name := tc.IR.NewDeclNameAt(pos, ir.OTYPE, u.LocalPkg.Lookup("Record"))
	record := u.NewNamed(name)
	name.SetType(record)
	name.SetTypecheck(1)
	record.SetUnderlying(u.NewStruct([]*types.Field{u.NewField(pos, u.LocalPkg.Lookup("Text"), u.Types[types.String])}))
	methodSym := u.LocalPkg.Lookup("M")
	record.SetMethods([]*types.Field{u.NewField(pos, methodSym, u.NewSignature(u.NewField(pos, nil, record), nil, nil))})
	iface := u.NewInterface([]*types.Field{u.NewField(pos, methodSym, u.NewSignature(u.NewField(pos, nil, u.FakeRecvType()), nil, nil))})
	c.NeedRuntimeType(record)
	c.WriteRuntimeTypes()
	descriptor := c.TypeLinksym(record)
	if !descriptor.OnList() || descriptor.TypeInfo().Type != record || descriptor.Size <= rt.StructType.Size() || binary.LittleEndian.Uint32(descriptor.P[rt.Type.OffsetOf("Hash"):]) != u.TypeHash(record) {
		t.Fatal("public type descriptor")
	}
	tab := c.ITabLsym(record, iface)
	address := c.ITabAddrAt(pos, record, iface)
	if !tab.OnList() || tab.ItabInfo().Type != record || tab.Size != rt.ITab.Size() || len(tab.R) != 3 || address.X.(*ir.LinksymOffsetExpr).Linksym != tab {
		t.Fatal("public ITab data")
	}
	conversion := tc.IR.NewConvExpr(pos, ir.OCONVIFACE, iface, tc.IR.NewNameAt(pos, u.LocalPkg.Lookup("value"), record))
	if c.ConvIfaceTypeWord(pos, conversion).(*ir.AddrExpr).X.(*ir.LinksymOffsetExpr).Linksym != tab {
		t.Fatal("public interface conversion type word")
	}

}

func TestNativeStaticDataPublicAPI(t *testing.T) {
	layout, _ := target.NewLayout("linux", "amd64")
	link := &gcbridge.Link{Arch: &gcbridge.LinkArch{Arch: &gcbridge.Arch{Name: "amd64", PtrSize: 8, RegSize: 8, ByteOrder: binary.LittleEndian}}}
	u, err := types.NewUniverseWithPositions(layout, &link.PosTable)
	if err != nil {
		t.Fatal(err)
	}
	u.LocalPkg = u.NewPkg("example/p", "p")
	graph := ir.NewContext(u, link)
	typecheck.NewContext(graph).InitUniverse()
	c := staticdata.NewContext(graph, objw.NewContext(u, link, "linux"))
	pos := position.XPos{}
	data := c.StringSym(pos, "hello")
	if string(data.P) != "hello" || data != c.StringSym(pos, "hello") {
		t.Fatal("public native string data")
	}
	n := graph.NewNameAt(pos, u.LocalPkg.Lookup("Text"), u.Types[types.String])
	n.Class = ir.PEXTERN
	c.InitConst(n, 0, graph.NewBasicLit(pos, n.Type(), constant.MakeString("hello")), 16)
	symbol := c.GlobalLinksym(n)
	if len(symbol.R) != 1 || symbol.R[0].Sym != data || binary.LittleEndian.Uint64(symbol.P[8:]) != 5 {
		t.Fatal("public native string initializer")
	}
	fn := graph.NewFunc(pos, pos, u.LocalPkg.Lookup("F"), u.NewSignature(nil, nil, nil))
	value := c.FuncLinksym(fn.Nname)
	if value.OnList() {
		t.Fatal("premature function value emission")
	}
	c.WriteFuncSyms()
	if !value.OnList() || len(value.R) != 1 || value.R[0].Sym != fn.Nname.Linksym() || value.R[0].Sym.ABI() != gcbridge.ABIInternal {
		t.Fatal("public native function value")
	}
}

// This consumer supplies native lookup hooks for its already-built local graph.
// Production noder bindings are exercised by the reader integration tests.
func TestPublicCallGraph(t *testing.T) {
	layout, _ := target.NewLayout("linux", "amd64")
	link := &gcbridge.Link{}
	u, err := types.NewUniverseWithPositions(layout, &link.PosTable)
	if err != nil {
		t.Fatal(err)
	}
	u.LocalPkg = u.NewPkg("example/p", "p")
	graph := ir.NewContext(u, link)
	tc := typecheck.NewContext(graph)
	tc.InitUniverse()
	pos := link.PosTable.XPos(position.Make(position.NewFile("p.go", "/p.go"), 10, 1))
	fn := graph.NewFunc(pos, pos, u.LocalPkg.Lookup("F"), u.NewSignature(nil, nil, nil))
	fn.Nname.Defn = fn
	tc.Target.Funcs = []*ir.Func{fn}
	visits := 0
	graph.VisitFuncsBottomUp(tc.Target.Funcs, func(fns []*ir.Func, recursive bool) {
		visits++
		if len(fns) != 1 || fns[0] != fn || recursive {
			t.Fatal("SCC")
		}
	})
	input, err := pgo.FromSerialized(strings.NewReader("GO PREPROFILE V1\nexample/p.F\nexample/p.F\n2 7\n"))
	if err != nil {
		t.Fatal(err)
	}
	c := pgoir.NewContext(tc)
	cleaned := false
	c.LookupFunc = func(string) (*ir.Func, error) { t.Fatal("local function should already be present"); return nil, nil }
	c.PostLookupCleanup = func() { cleaned = true }
	profile := c.NewProfile(input)
	key := input.NamedEdgeMap.ByWeight[0]
	heur := inlheur.NewContext(tc, c)
	fn.Body = ir.Nodes{graph.NewReturnStmt(pos, nil)}
	fn.Inl = &ir.Inline{Cost: 1}
	heur.AnalyzeFunc(fn, func(*ir.Func) { t.Fatal("unexpected closure") }, func(*ir.Func) int32 { return 80 }, 80)
	if inlheur.DeserializeFromString(fn.Inl.Properties) == nil {
		t.Fatal("public inline properties")
	}
	var oracle ir.ReassignOracle
	oracle.Init(fn)
	lit := graph.NewInt(pos, 42)
	if oracle.StaticValue(lit) != lit {
		t.Fatal("public reassignment oracle")
	}
	logger := logopt.NewContext()
	logger.Format = logopt.Json0
	logger.LogOpt(pos, "canInlineFunction", "inline", "F", fn.Inl.Cost)
	event := logger.Snapshot()[0]
	if event.Position() != pos || event.What() != "canInlineFunction" || event.Arguments()[0] != int32(1) {
		t.Fatal("public native log")
	}
	var node *pgoir.IRNode = profile.WeightedCG.IRNodes["example/p.F"]
	if visits != 1 || !cleaned || node.AST != fn || node.OutEdges[key].Weight != 7 || c.DirectCallee(fn.Nname) != fn {
		t.Fatal("public native graph")
	}
}

func TestPublicInlinability(t *testing.T) {
	link := &gcbridge.Link{}
	layout, err := target.NewLayout("linux", "amd64")
	if err != nil {
		t.Fatal(err)
	}
	u, err := types.NewUniverseWithPositions(layout, &link.PosTable)
	if err != nil {
		t.Fatal(err)
	}
	u.LocalPkg = u.NewPkg("example/public", "public")
	graph := ir.NewContext(u, link)
	tc := typecheck.NewContext(graph)
	tc.InitUniverse()
	fn := graph.NewFunc(u.AutogeneratedPos, u.AutogeneratedPos, u.LocalPkg.Lookup("F"), u.NewSignature(nil, nil, nil))
	fn.Body = ir.Nodes{graph.NewReturnStmt(fn.Pos(), nil)}
	fn.SetTypecheck(1)
	logger := logopt.NewContext()
	logger.Format = logopt.Json0
	c := inline.NewContext(tc, inlheur.NewContext(tc, pgoir.NewContext(tc)), logger)
	graph.Base.Flag.LowerL = 1
	c.CanInlineFuncs([]*ir.Func{fn}, nil)
	if fn.Inl == nil || len(logger.Snapshot()) != 1 {
		t.Fatal("public native inlining analysis")
	}
	var hash *gcbridge.HashDebug = graph.Base.NewHashDebug("pgo", "n", nil)
	graph.Base.PGOHash = hash
	if hash.MatchPkgFunc("p", "F", nil) {
		t.Fatal("public hash gate")
	}
}

func TestNativeCanSSAFromPublicModule(t *testing.T) {
	layout, err := target.NewLayout("linux", "amd64")
	if err != nil {
		t.Fatal(err)
	}
	link := &gcbridge.Link{}
	u, err := types.NewUniverseWithPositions(layout, &link.PosTable)
	if err != nil {
		t.Fatal(err)
	}
	graph := ir.NewContext(u, link)
	typecheck.NewContext(graph).InitUniverse()
	native := ssa.NewTypes(u)
	var comparison types.Cmp = native.Int.Compare(native.Int)
	if comparison != types.CMPeq || native.Int.Compare(nil) != types.CMPgt || (*types.Type)(nil).Compare(native.Int) != types.CMPlt || u.ByteType.Compare(u.Types[types.Uint8]) != types.CMPeq {
		t.Fatal("public native SSA type ordering")
	}

	if native.Int != u.Types[types.Int] || native.BytePtrPtr.Elem() != native.BytePtr {
		t.Fatal("public SSA types lost canonical identity")
	}
	copy := *native
	copy.Int = nil
	copy.SetTypPtrs()
	if copy.Int != native.Int {
		t.Fatal("copied public SSA types lost their universe")
	}
	slot := ssa.LocalSlot{Type: native.Int, Off: 8}
	var location ssa.Location = slot
	if (ssa.LocPair{location, nil}).String() != "<"+slot.String()+",nil>" {
		t.Fatal("public native SSA locations")
	}

	var op ssa.Op = ssa.OpAdd64
	var block ssa.BlockKind = ssa.BlockIf
	var assembly gcbridge.As = ssa.OpAMD64ADDQ.Asm()
	if op.String() != "Add64" || op.IsCall() || block.String() != "If" || assembly == 0 || !ssa.OpStaticCall.IsCall() {
		t.Fatal("public native SSA opcode identities and queries")
	}
	if ssa.SymRdWr != ssa.SymRead|ssa.SymWrite || op.SymEffect() != ssa.SymNone {
		t.Fatal("public native SSA symbol effects")
	}

	cfg := abi.NewContext(graph).NewABIConfig(2, 2, 0, 1)
	info := cfg.ABIAnalyzeTypes([]*types.Type{native.Int}, []*types.Type{native.Int})
	symbol := &gcbridge.LinkSymbol{Name: "example/p.F"}
	call := ssa.StaticAuxCall(symbol, info)
	var aux ssa.Aux = call
	var sym ssa.Sym = symbol
	result := call.LateExpansionResultType()
	if aux != call || sym != symbol || call.ABI() != cfg || cfg.Universe() != u || call.NArgs() != 1 || call.NResults() != 1 || result.FieldType(0) != native.Int || result.FieldType(1) != u.TypeMem {
		t.Fatal("public SSA call ABI ownership")
	}
	for _, other := range []*ssa.AuxCall{ssa.InterfaceAuxCall(info), ssa.ClosureAuxCall(info), ssa.OwnAuxCall(symbol, info)} {
		if other.ABIInfo() != info || other.TypeOfResult(0) != native.Int {
			t.Fatal("public call constructor")
		}
	}
	empty := ssa.ClosureAuxCall(cfg.ABIAnalyzeTypes(nil, nil))
	if empty.LateExpansionResultType() != u.TypeResultMem || fmt.Sprint(ssa.StringToAux("text")) != "text" {
		t.Fatal("public zero-result call or string aux")
	}

	i := u.Types[types.Int]
	if !ssa.CanSSA(u.NewSlice(i)) || ssa.CanSSA(u.NewArray(i, 2)) || ssa.MaxStruct != 4 {
		t.Fatal("public native SSA representation predicate")
	}
}

func TestNativeS390XAuxFromPublicModule(t *testing.T) {
	var cc ssa.S390XCCMask = 8
	var aux ssa.Aux = cc
	if got := aux.(ssa.S390XCCMask); got.String() != "Equal" || got.Inverse() != 7 || got.ReverseComparison() != 8 {
		t.Fatal("native condition mask")
	}
	r := ssa.NewS390XRotateParams(60, 3, 17)
	aux = r
	got := aux.(ssa.S390XRotateParams)
	if got.OutMask() != 0xf00000000000000f || got.RotateLeft(50).Amount != 3 {
		t.Fatal("native rotate parameters")
	}
	if got.OutMerge(0) != nil || got.InMerge(0) != nil {
		t.Fatal("empty native mask")
	}
}

func TestNativeSSAConfigAndGraphFromPublicModule(t *testing.T) {
	layout, err := target.NewLayout("linux", "amd64")
	if err != nil {
		t.Fatal(err)
	}
	link := &gcbridge.Link{Arch: &gcbridge.LinkArch{Arch: &gcbridge.Arch{Name: "amd64", PtrSize: 8, RegSize: 8, ByteOrder: binary.LittleEndian}}}
	u, err := types.NewUniverseWithPositions(layout, &link.PosTable)
	if err != nil {
		t.Fatal(err)
	}
	u.LocalPkg = u.NewPkg("example/ssa", "ssa")
	graph := ir.NewContext(u, link)
	tc := typecheck.NewContext(graph)
	tc.InitUniverse()
	rt := rttype.NewContext(objw.NewContext(u, link, "linux"))
	rt.Init()
	c := ssa.NewContext(abi.NewContext(graph), reflectdata.NewContext(tc, rt, staticdata.NewContext(graph, rt.Writer)), logopt.NewContext())
	c.GOAMD64 = 1
	cfg := c.NewConfig("amd64", *c.NewTypes(), link, true, false)
	var cache ssa.Cache
	cache.Reset()
	fn := graph.NewFunc(position.XPos{}, position.XPos{}, u.LocalPkg.Lookup("public"), u.NewSignature(nil, nil, nil))
	fn.DeclareParams(true)
	fe := &publicSplitFrontend{fn: fn}
	f := cfg.NewFunc(fe, &cache)
	f.Name = "public"
	f.Type = u.NewSignature(nil, nil, nil)
	f.ABISelf = cfg.ABI1
	b := f.NewBlock(ssa.BlockRet)
	f.Entry = b
	v := b.NewValue0(position.XPos{}, ssa.OpInitMem, u.TypeMem)
	b.SetControl(v)
	if cfg.Ctxt() != link || cfg.ABI1.Universe() != u || v.Block != b || b.Func != f || f.NumBlocks() != 2 || f.NumValues() != 2 {
		t.Fatal("public SSA graph identity lost")
	}
	if !strings.Contains(f.String(), "InitMem") {
		t.Fatal("native graph formatting unavailable")
	}
	n := graph.NewNameAt(position.XPos{}, u.LocalPkg.Lookup("s"), cfg.Types.String)
	n.Class = ir.PPARAM
	slot := &ssa.LocalSlot{N: n, Type: cfg.Types.String}
	ptr, length := f.SplitString(slot)
	ptr2, length2 := f.SplitString(slot)
	if ptr != ptr2 || length != length2 || ptr.Type != cfg.Types.BytePtr || length.Type != cfg.Types.Int || length.Off != 8 || fe.calls != 2 {
		t.Fatal("public native local-slot decomposition")
	}

	tmp := f.NewLocal(position.XPos{}, cfg.Types.Int64)
	if tmp.Curfn != fn || len(fn.Dcl) != 1 || fn.Dcl[0] != tmp || !tmp.NonMergeable() || !tmp.AutoTemp() {
		t.Fatal("public native temporary construction")
	}

	var _ *ssa.Value = v
	var _ *ssa.Block = b
	var _ *ssa.Func = f
}

// Parameter slots retain their Name and offset under the native Frontend contract.
type publicSplitFrontend struct {
	ssa.Frontend
	calls int
	fn    *ir.Func
}

func (fe *publicSplitFrontend) SplitSlot(parent *ssa.LocalSlot, suffix string, offset int64, typ *types.Type) ssa.LocalSlot {
	fe.calls++
	return ssa.LocalSlot{N: parent.N, Type: typ, Off: parent.Off + offset}
}

func (fe *publicSplitFrontend) Func() *ir.Func { return fe.fn }

package noder_test

import (
	"encoding/binary"
	"fmt"
	"go-backend-ssa/deadlocals"
	"go-backend-ssa/escape"
	"go-backend-ssa/gcbridge"
	"go-backend-ssa/ir"
	"go-backend-ssa/loopvar"
	"go-backend-ssa/noder"
	"go-backend-ssa/objw"
	"go-backend-ssa/pkginit"
	"go-backend-ssa/reflectdata"
	"go-backend-ssa/rttype"
	"go-backend-ssa/slice"
	"go-backend-ssa/ssagen"
	"go-backend-ssa/staticdata"
	"go-backend-ssa/staticinit"
	"go-backend-ssa/syntax"
	"go-backend-ssa/target"
	"go-backend-ssa/typecheck"
	"go-backend-ssa/types"
	"go-backend-ssa/types2"
	"strings"
)

func ExampleNewContext() {
	checker := types2.NewContext()
	info := &types2.Info{
		StoreTypesInSyntax: true,
		Defs:               map[*syntax.Name]types2.Object{},
		Uses:               map[*syntax.Name]types2.Object{},
		Instances:          map[*syntax.Name]types2.Instance{},
		Selections:         map[*syntax.SelectorExpr]*types2.Selection{},
		Implicits:          map[syntax.Node]types2.Object{},
		Scopes:             map[syntax.Node]*types2.Scope{},
		FileVersions:       map[*syntax.PosBase]string{},
	}
	layout, err := target.NewLayout("linux", "amd64")
	if err != nil {
		panic(err)
	}
	link := &gcbridge.Link{Pathname: "/src", Pkgpath: "example/p", Arch: &gcbridge.LinkArch{Arch: &gcbridge.Arch{Name: "amd64", PtrSize: 8, RegSize: 8, ByteOrder: binary.LittleEndian}}}
	universe, err := types.NewUniverseWithPositions(layout, &link.PosTable)
	if err != nil {
		panic(err)
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
	writer, err := noder.NewContext(checker, tc)
	if err != nil {
		panic(err)
	}
	writer.GOOS = "linux"
	parsed, err := writer.ParseFile("id.go", strings.NewReader("package p;func Id[T any](x T)T{return x};var X=Id(1);func Each(y func(int)bool){y(1)}\n//go:noinline\nfunc Sum()(n int){for x:=range Each{n+=x};return}"))
	if err != nil {
		panic(err)
	}
	files := []*syntax.File{parsed.Syntax()}
	config := types2.Config{Context: checker, GoVersion: "go1.27", Sizes: checker.SizesFor("gc", "amd64")}
	pkg, err := config.Check("example/p", files, info)
	if err != nil {
		panic(err)
	}
	closures, err := writer.RewriteRangeFuncs(pkg, info, files)
	if err != nil {
		panic(err)
	}
	stub, err := writer.WriteParsedPackage(pkg, info, []*noder.ParsedFile{parsed}, closures)
	if err != nil {
		panic(err)
	}
	ow := objw.NewContext(universe, link, "linux")
	rt := rttype.NewContext(ow)
	rt.Init()
	writer.ReflectData = reflectdata.NewContext(tc, rt, staticdata.NewContext(graph, ow))
	writer.StaticInit = staticinit.NewContext(tc, writer.ReflectData)
	target := writer.ReadLocalPackage(stub)
	writer.Flag.LowerL = 1
	writer.Interleaved.DevirtualizeAndInlinePackage(target, nil)
	writer.MakeWrappers(target)
	capture := loopvar.NewContext(tc, writer.Inline.Log)
	for _, fn := range target.Funcs {
		capture.ForCapture(fn)
	}
	pkginit.NewContext(writer).MakeTask()
	ssagen.NewContext(tc).NewSymABIs().GenABIWrappers()
	deadlocals.NewContext(graph).Funcs(target.Funcs)
	analysis := escape.NewContext(tc, writer.Inline.Log)
	analysis.Funcs(target.Funcs)
	graph.Debug.VariableMakeThreshold = 64
	slice.NewContext(analysis, writer.ReflectData).Funcs(target.Funcs)
	sum, err := writer.LookupFunc("example/p.Sum")
	if err != nil {
		panic(err)
	}
	fmt.Println(len(stub) > 0, pkg.Scope().Lookup("X").Type())
	fmt.Println(target == tc.Target, len(sum.Body) > 0, sum.Pragma&ir.Noinline != 0)
	// Output:
	// true int
	// true true true
}

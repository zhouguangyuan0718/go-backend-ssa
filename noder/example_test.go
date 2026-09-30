package noder_test

import (
	"encoding/binary"
	"fmt"
	"go-backend-ssa/bloop"
	"go-backend-ssa/coverage"
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
	"go-backend-ssa/target"
	"go-backend-ssa/typecheck"
	"go-backend-ssa/types"
	"go-backend-ssa/types2"
	"strings"
)

func ExampleNewContext() {
	checker := types2.NewContext()
	layout, err := target.NewLayout("linux", "amd64")
	if err != nil {
		panic(err)
	}
	link := &gcbridge.Link{TargetOS: "linux", Pathname: "/src", Pkgpath: "example/p", Arch: &gcbridge.LinkArch{Arch: &gcbridge.Arch{Name: "amd64", PtrSize: 8, RegSize: 8, ByteOrder: binary.LittleEndian}}}
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
	writer.BindImports(noder.ObjectConfig{GOOS: "linux", GOARCH: "amd64", GOAMD64: 1, Version: "go1.27.1"}, "linux", func(path string) (noder.DependencyFile, error) {
		return nil, fmt.Errorf("unexpected dependency %s", path)
	})
	parsed, err := writer.ParseFile("id.go", strings.NewReader("package p;func Id[T any](x T)T{return x};var X=Id(1);func Each(y func(int)bool){y(1)}\n//go:noinline\nfunc Sum()(n int){for x:=range Each{n+=x};return}"))
	if err != nil {
		panic(err)
	}
	ow := objw.NewContext(universe, link, "linux")
	rt := rttype.NewContext(ow)
	rt.Init()
	writer.ReflectData = reflectdata.NewContext(tc, rt, staticdata.NewContext(graph, ow))
	writer.StaticInit = staticinit.NewContext(tc, writer.ReflectData)
	target, err := writer.CompileParsedPackage([]*noder.ParsedFile{parsed})
	if err != nil {
		panic(err)
	}
	writer.Flag.LowerL = 1
	coverage.NewContext(tc).Fixup()
	bloop.NewContext(tc).Walk(target)
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
	fmt.Println(writer.ExportAvailable(), universe.LocalPkg.Lookup("X").Def.(*ir.Name).Type())
	fmt.Println(target == tc.Target, len(sum.Body) > 0, sum.Pragma&ir.Noinline != 0)
	// Output:
	// true int
	// true true true
}

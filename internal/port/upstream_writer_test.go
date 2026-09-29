package port

import (
	"go/token"
	"path"
	"strings"
	"testing"
)

// Add a full native writer reference to the existing raw checker reference.
// Algorithms retain their package state; only test driver inputs/diagnostics
// are substituted. The library never depends on this reference package.
func prepareUpstreamWriter(t *testing.T, s *Source, dir string) {
	t.Helper()
	mapping := Migration().importMap(s.Pin.TargetModule)
	for _, p := range []string{"cmd/compile/internal/rangefunc", "cmd/compile/internal/base", "cmd/compile/internal/types", "cmd/compile/internal/types2", "cmd/compile/internal/syntax", "cmd/internal/objabi", "internal/buildcfg", "internal/pkgbits"} {
		mapping[p] = "go-backend-ssa/internal/reference/" + path.Base(p)
	}
	mapping["internal/types/errors"] = "go-backend-ssa/internal/reference/errors"
	relocate := func(text string) string {
		for from, to := range mapping {
			text = strings.ReplaceAll(text, `"`+from+`"`, `"`+to+`"`)
		}
		return strings.ReplaceAll(text, "src.NoXPos", "src.XPos{}")
	}
	for _, u := range rangefuncMigration() {
		data, err := s.Read(u.Source)
		if err != nil {
			t.Fatal(err)
		}
		if path.Base(path.Dir(u.Source)) == "rangefunc" {
			writeTestFile(t, dir, "internal/reference/rangefunc/rewrite.go", relocate(string(data)))
		}
	}
	for _, u := range noderMigration() {
		// This harness compares writer bytes using the original types2 graph.
		// The native reader uses compiler types/IR and is compared separately
		// through the full-toolchain overlay in upstream_ir_test.go.
		if path.Base(u.Source) == "helpers.go" {
			u.WholeFile = false
			u.Functions = []string{"idealType", "isTypeParam", "isNotInHeap"}
		}
		if path.Base(u.Source) == "types.go" {
			u.WholeFile = false
			u.Functions = []string{"deref2"}
			u.Context.Fields = nil
		}
		if path.Base(u.Source) == "noder.go" {
			u.Functions = []string{"trimFilename", "checkEmbed"}
			delete(u.Context.Fields, "renameinitgen")
		}
		if path.Base(u.Source) == "unified.go" {
			u.Context.Fields = nil
		}
		if path.Base(u.Source) == "reader.go" {
			continue
		}
		if path.Dir(u.Source) != "src/cmd/compile/internal/noder" {
			continue
		}
		if path.Base(u.Source) == "unified.go" {
			u.Functions = append(u.Functions, "freePackage")
		}
		data, err := s.Read(u.Source)
		if err != nil {
			t.Fatal(err)
		}
		fs := token.NewFileSet()
		f, err := selectFile(fs, u.Source, data, u)
		if err != nil {
			t.Fatal(err)
		}
		pruneImports(f)
		writeTestFile(t, dir, "internal/reference/noder/"+path.Base(u.Source), relocate(nodeText(fs, f)))
	}
	for _, u := range []Unit{
		{Source: "src/cmd/internal/objabi/line.go", Functions: []string{"AbsFile", "ApplyRewrites", "applyRewrite", "hasPathPrefix"}},
		{Source: "src/cmd/compile/internal/types/fmt.go", Functions: []string{"SplitVargenSuffix"}},
		{Source: "src/cmd/compile/internal/types/goversion.go", Functions: []string{"AllowsGoVersion"}, Declarations: []string{"lang"}, Context: &ContextRule{Fields: map[string]string{"langWant": "langWant"}}},
	} {
		data, err := s.Read(u.Source)
		if err != nil {
			t.Fatal(err)
		}
		fs := token.NewFileSet()
		f, err := selectFile(fs, u.Source, data, u)
		if err != nil {
			t.Fatal(err)
		}
		pruneImports(f)
		writeTestFile(t, dir, "internal/reference/"+path.Base(path.Dir(u.Source))+"/"+path.Base(u.Source), relocate(nodeText(fs, f)))
	}
	for _, name := range sortedKeys(s.Blobs) {
		if path.Dir(name) == "src/internal/pkgbits" && strings.HasSuffix(name, ".go") && !strings.HasSuffix(name, "_test.go") {
			data, err := s.Read(name)
			if err != nil {
				t.Fatal(err)
			}
			writeTestFile(t, dir, "internal/reference/pkgbits/"+path.Base(name), string(data))
		}
	}
	writeTestFile(t, dir, "internal/reference/base/driver.go", `package base
import("fmt";native "go-backend-ssa/internal/base";"go-backend-ssa/internal/cmd/obj";"go-backend-ssa/internal/cmd/src";"go-backend-ssa/internal/reference/errors")
var Flag native.CmdFlags
var Debug native.DebugFlags
var Ctxt *obj.Link
const CompilerBootstrap=false
var Diagnostics []string
func Assert(b bool){if !b{panic("assertion failed")}}
func Assertf(b bool,f string,a ...any){if !b{panic(fmt.Sprintf(f,a...))}}
func Fatalf(f string,a ...any){panic(fmt.Sprintf(f,a...))}
func FatalfAt(p src.XPos,f string,a ...any){panic(fmt.Sprintf(f,a...))}
func ErrorfAt(p src.XPos,code errors.Code,f string,a ...any){Diagnostics=append(Diagnostics,fmt.Sprintf(f,a...))}
`)
	writeTestFile(t, dir, "internal/reference/buildcfg/input.go", `package buildcfg
var GOARCH string
var GOROOT string
`)
	writeTestFile(t, dir, "internal/reference/noder/driver.go", `package noder
import("fmt";"go-backend-ssa/internal/cmd/obj";"go-backend-ssa/internal/reference/base";"go-backend-ssa/internal/reference/buildcfg";"go-backend-ssa/internal/reference/rangefunc";"go-backend-ssa/internal/reference/syntax";"go-backend-ssa/internal/reference/types2")
var checkedPkg *types2.Package
var checkedInfo *types2.Info
var checkedRange map[*syntax.FuncLit]bool
func checkFiles(posMap,[]*noder)(*types2.Package,*types2.Info,map[*syntax.FuncLit]bool){return checkedPkg,checkedInfo,checkedRange}
func WriteChecked(pkg *types2.Package,info *types2.Info,files []*syntax.File,arch string,frames,checks int)string{
 checkedPkg,checkedInfo=pkg,info
 base.Ctxt=&obj.Link{Pathname:"/src"};base.Flag.Lang="go1.27";base.Flag.Complete=true;base.Debug.SyncFrames=frames;base.Debug.RangeFuncCheck=checks;base.Diagnostics=nil;buildcfg.GOARCH=arch
 checkedRange=rangefunc.Rewrite(pkg,info,files)
 nodes:=make([]*noder,len(files));for i,f:=range files{nodes[i]=&noder{file:f}}
 data:=writePkgStub(posMap{},nodes)
 if len(base.Diagnostics)!=0{panic(fmt.Sprint(base.Diagnostics))}
 return data
}
`)
	header := `package compare_test
import("fmt";"strings";"testing";"go-backend-ssa/internal/cmd/obj";"go-backend-ssa/internal/cmd/sys";"go-backend-ssa/internal/types";"go-backend-ssa/internal/typecheck";"go-backend-ssa/internal/ir";pc "go-backend-ssa/internal/types2";ps "go-backend-ssa/internal/syntax";pn "go-backend-ssa/internal/noder";rc "go-backend-ssa/internal/reference/types2";rs "go-backend-ssa/internal/reference/syntax";rn "go-backend-ssa/internal/reference/noder")
`
	template := `
type NAMEImports struct{unsafe,dep *TC.Package}
func(i NAMEImports)Import(p string)(*TC.Package,error){switch p{case "unsafe":return i.unsafe,nil;case "dep":return i.dep,nil};return nil,fmt.Errorf("unknown import %s",p)}
func NAME(t *testing.T,source,arch string,frames,checks int)string{
 t.Helper()
 ctxt:=TC.NewContext()
 conf:=TC.Config{Context:ctxt,GoVersion:"go1.27",Sizes:SIZES}
 depfile,err:=SYN.Parse(SYN.NewFileBase("dep.go"),strings.NewReader("package dep;type Box[T any]struct{Value T};func Id[T any](x T)T{return x}"),nil,nil,SYN.CheckBranches);if err!=nil{t.Fatal(err)}
 dep,err:=conf.Check("dep",[]*SYN.File{depfile},nil);if err!=nil{t.Fatal(err)}
 conf.Importer=NAMEImports{UNSAFE,dep}
 file,err:=SYN.Parse(SYN.NewFileBase("input.go"),strings.NewReader(source),nil,nil,SYN.CheckBranches);if err!=nil{t.Fatal(err)}
 info:=&TC.Info{StoreTypesInSyntax:true,Defs:map[*SYN.Name]TC.Object{},Uses:map[*SYN.Name]TC.Object{},Selections:map[*SYN.SelectorExpr]*TC.Selection{},Implicits:map[SYN.Node]TC.Object{},Scopes:map[SYN.Node]*TC.Scope{},Instances:map[*SYN.Name]TC.Instance{},FileVersions:map[*SYN.PosBase]string{}}
 pkg,err:=conf.Check("example/p",[]*SYN.File{file},info);if err!=nil{t.Fatal(err)}
 WRITE
}
`
	library := strings.NewReplacer("NAME", "writePorted", "TC", "pc", "SYN", "ps", "SIZES", `ctxt.SizesFor("gc",arch)`, "UNSAFE", "ctxt.Unsafe", "WRITE", `ptr:=8;if arch=="386"{ptr=4};link:=&obj.Link{Pathname:"/src",Arch:&obj.LinkArch{Arch:&sys.Arch{Name:arch,PtrSize:ptr,RegSize:ptr}}};u:=types.NewUniverseWithPositions(ptr,ptr,1<<50,&link.PosTable);graph:=ir.NewContext(u,link);tc:=typecheck.NewContext(graph);tc.InitUniverse();graph.Base.Flag.Lang="go1.27";graph.Base.Flag.Complete=true;c,err:=pn.NewContext(ctxt,tc);if err!=nil{t.Fatal(err)};c.Debug.SyncFrames=frames;c.Debug.RangeFuncCheck=checks;closures,err:=c.RewriteRangeFuncs(pkg,info,[]*ps.File{file});if err!=nil{t.Fatal(err)};data,err:=c.WritePackage(pkg,info,[]*ps.File{file},closures);if err!=nil{t.Fatal(err)};return data`).Replace(template)
	reference := strings.NewReplacer("NAME", "writeReference", "TC", "rc", "SYN", "rs", "SIZES", `rc.SizesFor("gc",arch)`, "UNSAFE", "rc.Unsafe", "WRITE", `return rn.WriteChecked(pkg,info,[]*rs.File{file},arch,frames,checks)`).Replace(template)
	tests := `
func TestNativeWriterBytes(t *testing.T){
 sources:=[]string{
  "package p;func Add(x,y int)int{for i:=0;i<y;i++{if i%2==0{x+=i}};return x}",
  "package p;type Box[T any]struct{V T};func Id[T any](x T)T{return x};var X=Id(Box[int]{V:3})",
  "package p;type Box[T any]struct{V T};type Alias[T any]=Box[T];var X Alias[int]",
  "package p;func F(x int)func()int{return func()int{x++;return x}}",
  "package p;type T int;func(t T)M()int{return int(t)};var I interface{M()int}=T(1)",
  "package p;func F(x any)(v int){switch x:=x.(type){case int:return x;case string:return len(x)};return 0}",
  "package p;import \"unsafe\";const Word=unsafe.Sizeof(uintptr(0));type T struct{A byte;B int};const Offset=unsafe.Offsetof(T{}.B)",
  "package p;var A=F();var B=A+2;func F()int{return 1}",
  "package p;func F(xs []int)(n int){for i,x:=range xs{n+=i*x};for i:=range 4{n+=i};return}",
  "package p;func F(m map[string]int,ch chan int){defer func(){recover()}();select{case ch<-m[\"a\"]:default:};go func(){ch<-3}()}",
  "package p;import \"dep\";var X=dep.Id(dep.Box[int]{Value:3})",
  "package p\n//line generated.go:40:3\nfunc F()string{return \"hello\"}\n",
 }
 for i,source:=range sources{for _,arch:=range []string{"386","amd64"}{for _,frames:=range []int{-1,0}{t.Run(fmt.Sprintf("%d/%s/%d",i,arch,frames),func(t *testing.T){want,got:=writeReference(t,source,arch,frames,1),writePorted(t,source,arch,frames,1);if got!=want{at:=0;for at<len(got)&&at<len(want)&&got[at]==want[at]{at++};t.Fatalf("native stub bytes differ: offset %d, ported %d bytes, reference %d bytes",at,len(got),len(want))}})}}}
}
`
	writeTestFile(t, dir, "internal/compare/writer_test.go", header+library+reference+tests+rangefuncByteTests)
}

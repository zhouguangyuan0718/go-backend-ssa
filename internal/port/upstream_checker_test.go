package port

import (
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"testing"
)

// Compile the complete original checker and parser alongside the migrated
// library. The reference changes import paths only, retaining globals and init.
func testUpstreamChecker(t *testing.T, s *Source, r *Rendered, root string) {
	t.Helper()
	dir := t.TempDir()
	for name, data := range r.Files {
		writeTestFile(t, dir, name, string(data))
	}
	handwritten := append([]string(nil), Migration().SupportFiles...)
	handwritten = append(handwritten, "types2/types2.go", "syntax/syntax.go", "syntax/aliases.go")
	for _, u := range Migration().Units {
		if u.Context != nil && u.Context.Adapter != "" {
			handwritten = append(handwritten, u.adapterPath())
		}
	}
	for _, name := range handwritten {
		data, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			t.Fatal(err)
		}
		writeTestFile(t, dir, name, string(data))
	}
	mapping := map[string]string{
		"cmd/compile/internal/types2": "go-backend-ssa/internal/reference/types2",
		"cmd/compile/internal/syntax": "go-backend-ssa/internal/reference/syntax",
		"internal/types/errors":       "go-backend-ssa/internal/reference/errors",
		"internal/goversion":          "go-backend-ssa/internal/reference/goversion",
	}
	for _, name := range sortedKeys(s.Blobs) {
		pkg := strings.TrimPrefix(path.Dir(name), "src/")
		dest := mapping[pkg]
		if dest == "" || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		if pkg == "internal/types/errors" && path.Base(name) != "codes.go" && path.Base(name) != "code_string.go" {
			continue
		}
		data, err := s.Read(name)
		if err != nil {
			t.Fatal(err)
		}
		text := string(data)
		for from, to := range mapping {
			text = strings.ReplaceAll(text, "\""+from+"\"", "\""+to+"\"")
		}
		writeTestFile(t, dir, strings.TrimPrefix(dest, "go-backend-ssa/")+"/"+path.Base(name), text)
	}
	writeTestFile(t, dir, "go.mod", "module go-backend-ssa\n\ngo 1.27.1\n")
	header := `package compare
import("fmt";"reflect";"sort";"strings";"testing";ported "go-backend-ssa/types2";ps "go-backend-ssa/syntax";original "go-backend-ssa/internal/reference/types2";rs "go-backend-ssa/internal/reference/syntax")
`
	template := `
type NAMEImporter struct{unsafe *API.Package}
func(i NAMEImporter)Import(p string)(*API.Package,error){if p=="unsafe"{return i.unsafe,nil};return nil,fmt.Errorf("unexpected import %s",p)}
func NAME(source,arch,version string)[]string{
 var out []string
 f,err:=SYNTAX.Parse(SYNTAX.NewFileBase("input.go"),strings.NewReader(source),func(err error){out=append(out,"parse:"+err.Error())},nil,SYNTAX.CheckBranches)
 if err!=nil{return append(out,"parse-first:"+err.Error())}
 SETUP
 info:=&API.Info{Types:map[SYNTAX.Expr]API.TypeAndValue{},Defs:map[*SYNTAX.Name]API.Object{},Instances:map[*SYNTAX.Name]API.Instance{},StoreTypesInSyntax:true}
 conf.Error=func(err error){e:=err.(API.Error);out=append(out,fmt.Sprintf("error:%s:%s:%s:%t",e.Pos,e.Code,e.Msg,e.Soft))}
 p,err:=conf.Check("example/p",[]*SYNTAX.File{f},info)
 if err!=nil{out=append(out,"first:"+err.Error())}
 out=append(out,fmt.Sprintf("package:%s:%s:%s:%t",p.Path(),p.Name(),p.GoVersion(),p.Complete()))
 for _,name:=range p.Scope().Names(){obj:=p.Scope().Lookup(name);typ:="<nil>";if obj.Type()!=nil{typ=obj.Type().String()};out=append(out,"scope:"+name+":"+typ);if c,ok:=obj.(*API.Const);ok{out=append(out,"const:"+name+":"+c.Val().ExactString())}}
 for id,obj:=range info.Defs{if obj==nil{continue};typ:="<nil>";if obj.Type()!=nil{typ=obj.Type().String()};out=append(out,fmt.Sprintf("def:%s:%s:%s",id.Pos(),id.Value,typ))}
 for e,tv:=range info.Types{typ:="<nil>";if tv.Type!=nil{typ=tv.Type.String()};val:="";if tv.Value!=nil{val=tv.Value.ExactString()};out=append(out,fmt.Sprintf("expr:%s:%s:%s:%s:%t:%t:%t:%t",e.Pos(),SYNTAX.String(e),typ,val,tv.IsType(),tv.IsValue(),tv.Addressable(),e.GetTypeInfo().Type==tv.Type))}
 for id,instance:=range info.Instances{out=append(out,fmt.Sprintf("instance:%s:%s:%s",id.Pos(),id.Value,instance.Type.String()))}
 for i,init:=range info.InitOrder{out=append(out,fmt.Sprintf("init:%d:%s",i,init.String()))}
 out=append(out,"empty-tuple:"+(*API.Tuple)(nil).String())
 sort.Strings(out);return out
}
`
	port := strings.NewReplacer("NAME", "snapshotPort", "API", "ported", "SYNTAX", "ps", "SETUP", `c:=ported.NewContext();conf:=ported.Config{Context:c,Sizes:c.SizesFor("gc",arch),GoVersion:version,Importer:snapshotPortImporter{c.Unsafe}}`).Replace(template)
	reference := strings.NewReplacer("NAME", "snapshotReference", "API", "original", "SYNTAX", "rs", "SETUP", `conf:=original.Config{Sizes:original.SizesFor("gc",arch),GoVersion:version,Importer:snapshotReferenceImporter{original.Unsafe}}`).Replace(template)
	tests := `
func TestDifferential(t *testing.T){
 sources:=[]string{
  "package p;func Add(x,y int)int{for i:=0;i<y;i++{if i%2==0{x+=i}};return x}",
  "package p;type Box[T any]struct{V T};func Id[T any](x T)T{return x};var X=Id(Box[int]{V:1})",
  "package p;type Box[T any]struct{V T};type A[T any]=Box[T];var X A[string]",
  "package p;type N int;func(n N)M()int{return int(n)};var X interface{M()int}=N(1)",
  "package p;type Node struct{Next *Node};func(*Node)M(){};var X interface{M()}=(*Node)(nil)",
  "package p;import \"unsafe\";const Word=unsafe.Sizeof(uintptr(0));type S struct{A byte;B int};const Offset=unsafe.Offsetof(S{}.B)",
  "package p;func Each(yield func(int)bool){yield(1)};func F(){for x:=range Each{_=x}}",
  "package p;type A=[]int;var X A;func F(x any)(int,bool){v,ok:=x.(int);return v,ok}",
  "package p;var X int=\"bad\";func F()int{return Missing}",
  "package p;var A=B;var B=A",
  "package p;const X int=1<<40",
  "package p;type Bad interface{~[]int};func F[T Bad](x T){_=x==x}",
 }
 for i,source:=range sources{for _,arch:=range []string{"386","amd64"}{for _,version:=range []string{"go1.17","go1.27"}{t.Run(fmt.Sprintf("%d/%s/%s",i,arch,version),func(t *testing.T){want,got:=snapshotReference(source,arch,version),snapshotPort(source,arch,version);if !reflect.DeepEqual(got,want){t.Fatalf("source: %s\nported:\n%s\noriginal:\n%s",source,strings.Join(got,"\n"),strings.Join(want,"\n"))}})}}}
}
`
	writeTestFile(t, dir, "internal/compare/checker_test.go", header+port+reference+tests)
	cmd := exec.Command("go", "test", "-race", "./internal/compare")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOWORK=off", "GOTOOLCHAIN=go1.27.1")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("checker differential at %s: %v\n%s", s.Pin.Tag, err, output)
	}
	t.Logf("complete checker source/type/diagnostic differential at %s (48 cases):\n%s", s.Pin.Tag, output)
}

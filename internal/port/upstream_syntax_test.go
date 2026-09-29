package port

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path"
	"strings"
	"testing"
)

func testUpstreamSyntax(t *testing.T, s *Source, r *Rendered) {
	t.Helper()
	dir := t.TempDir()
	for name, data := range r.Files {
		if strings.HasPrefix(name, "internal/syntax/") {
			writeTestFile(t, dir, name, string(data))
		}
	}
	// Compile the complete original parser beside the port, including its globals.
	prefix := "src/cmd/compile/internal/syntax/"
	for _, name := range sortedKeys(s.Blobs) {
		if !strings.HasPrefix(name, prefix) || !strings.HasSuffix(name, ".go") {
			continue
		}
		relative := strings.TrimPrefix(name, prefix)
		data, err := s.Read(name)
		if err != nil {
			t.Fatal(err)
		}
		if strings.HasPrefix(relative, "testdata/") {
			writeTestFile(t, dir, "internal/syntax/"+relative, string(data))
			continue
		}
		if strings.Contains(relative, "/") {
			continue
		}
		if !strings.HasSuffix(relative, "_test.go") {
			writeTestFile(t, dir, "internal/reference/syntax/"+relative, string(data))
			continue
		}
		fs := token.NewFileSet()
		f, err := parser.ParseFile(fs, name, data, parser.ParseComments)
		if err != nil {
			t.Fatal(err)
		}
		comments := ast.NewCommentMap(fs, f, f.Comments)
		var decls []ast.Decl
		for _, d := range f.Decls {
			if fn, ok := d.(*ast.FuncDecl); ok {
				// These two tests depend on the toolchain's internal testenv. Their
				// environment/performance concerns are not parser semantic regressions.
				if fn.Name.Name == "TestStdLib" || fn.Name.Name == "TestUnpackListExprAllocs" {
					continue
				}
				if fn.Name.Name == "TestSyntaxErrors" {
					var body []ast.Stmt
					for _, stmt := range fn.Body.List {
						if expr, ok := stmt.(*ast.ExprStmt); ok {
							if call, ok := expr.X.(*ast.CallExpr); ok {
								if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
									if id, ok := sel.X.(*ast.Ident); ok && id.Name == "testenv" && sel.Sel.Name == "MustHaveGoBuild" {
										continue
									}
								}
							}
						}
						body = append(body, stmt)
					}
					fn.Body.List = body
				}
			}
			decls = append(decls, d)
		}
		f.Decls = decls
		f.Comments = comments.Filter(f).Comments()
		out, err := transform(fs, f, nil, Unit{Source: name}, nil)
		if err != nil {
			t.Fatal(err)
		}
		writeTestFile(t, dir, "internal/syntax/"+path.Base(name), string(out))
	}
	// Keep the original file-reading convenience in test code only. Production
	// Parse uses an explicit Reader and never opens a path or consults the host.
	data, err := s.Read(prefix + "syntax.go")
	if err != nil {
		t.Fatal(err)
	}
	fs := token.NewFileSet()
	f, err := selectFile(fs, prefix+"syntax.go", data, Unit{Functions: []string{"ParseFile"}})
	if err != nil {
		t.Fatal(err)
	}
	out, err := transform(fs, f, nil, Unit{Source: prefix + "syntax.go"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, dir, "internal/syntax/adapter_parsefile_test.go", string(out))
	writeTestFile(t, dir, "go.mod", "module go-backend-ssa\n\ngo 1.27.1\n")
	writeTestFile(t, dir, "compare_test.go", syntaxComparison)
	cmd := exec.Command("go", "test", "-race", "-v", ".", "./internal/syntax")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOWORK=off", "GOTOOLCHAIN=go1.27.1")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("upstream syntax (%s): %v\n%s", s.Pin.Tag, err, output)
	}
	t.Logf("upstream syntax regressions and AST/error differential at %s:\n%s", s.Pin.Tag, output)
}

const syntaxComparison = `package comparison_test
import("bytes";"fmt";"os";"path/filepath";"reflect";"regexp";"sort";"strings";"testing"; native "go-backend-ssa/internal/reference/syntax"; library "go-backend-ssa/internal/syntax")
type snapshot struct{Tree,Dump,Pragmas,Positions string;Errors []string;Failed bool}
func original(name,source string)snapshot{
 var out snapshot
 f,err:=native.Parse(native.NewFileBase(name),strings.NewReader(source),func(e error){out.Errors=append(out.Errors,e.Error())},func(pos native.Pos,blank bool,text string,current native.Pragma)native.Pragma{out.Pragmas+=fmt.Sprint(pos,blank,text,current);return text},native.CheckBranches)
 out.Failed=err!=nil;sort.Strings(out.Errors)
 if f!=nil{var printed,dump bytes.Buffer;native.Fprint(&printed,f,0);native.Fdump(&dump,f);out.Tree=printed.String();out.Dump=normalizeDump(dump.String());if len(out.Errors)==0{native.Inspect(f,func(n native.Node)bool{if n!=nil{out.Positions+=fmt.Sprintf("%T@%v;",n,n.Pos())};return true})}};return out
}
func migrated(name,source string)snapshot{
 var out snapshot
 f,err:=library.Parse(library.NewFileBase(name),strings.NewReader(source),func(e error){out.Errors=append(out.Errors,e.Error())},func(pos library.Pos,blank bool,text string,current library.Pragma)library.Pragma{out.Pragmas+=fmt.Sprint(pos,blank,text,current);return text},library.CheckBranches)
 out.Failed=err!=nil;sort.Strings(out.Errors)
 if f!=nil{var printed,dump bytes.Buffer;library.Fprint(&printed,f,0);library.Fdump(&dump,f);out.Tree=printed.String();out.Dump=normalizeDump(dump.String());if len(out.Errors)==0{library.Inspect(f,func(n library.Node)bool{if n!=nil{out.Positions+=fmt.Sprintf("%T@%v;",n,n.Pos())};return true})}};return out
}
// Upstream Fdump prints raw addresses for SliceExpr's fixed array of Expr.
// Normalize only those address slots; separately compare every walked node's
// kind and full source position, plus the complete printed expression tree.
func normalizeDump(s string)string{
 lines:=strings.Split(s,"\n");addresses:=map[string]string{};rx:=regexp.MustCompile("0x[0-9a-f]+")
 for i,line:=range lines{if strings.Contains(line,"Index: ["){lines[i]=rx.ReplaceAllStringFunc(line,func(p string)string{if addresses[p]==""{addresses[p]=fmt.Sprintf("ptr%d",len(addresses)+1)};return addresses[p]})}}
 return strings.Join(lines,"\n")
}
func TestNativeBranchHandlerContract(t *testing.T){
 const src="package p;func f(){break}"
 for name,run:=range map[string]func(){"upstream":func(){native.Parse(nil,strings.NewReader(src),nil,nil,native.CheckBranches)},"ported":func(){library.Parse(nil,strings.NewReader(src),nil,nil,library.CheckBranches)}}{t.Run(name,func(t *testing.T){defer func(){if recover()==nil{t.Fatal("expected the pinned nil branch-handler behavior")}}();run()})}
 count:=0;f,err:=native.Parse(nil,strings.NewReader(src),func(error){count++},nil,native.CheckBranches);if f==nil||err!=nil||count!=1{t.Fatal("pinned branch callback contract changed",f,err,count)}
}
func TestParserDifferential(t *testing.T){
 files,err:=filepath.Glob("internal/syntax/testdata/*.go");if err!=nil||len(files)==0{t.Fatal(files,err)}
 cases:=map[string]string{
 "labels.go":"package p; func f(){Outer: { }; for {break Outer}}",
 "generic.go":"package p; type Box[T ~int] struct{V T}; func Map[T any](v T)T{return v}",
 "directive.go":"//go:build go1.27\npackage p\n//go:noinline\nfunc f(){}\n//line mapped.go:90:3\nvar x int\n",
 "bad.go":"package p; func f(){goto L;var x int;L: return;break }",
 }
 for _,file:=range files{data,err:=os.ReadFile(file);if err!=nil{t.Fatal(err)};cases[file]=string(data)}
 for name,source:=range cases{t.Run(name,func(t *testing.T){want:=original(name,source);got:=migrated(name,source);if !reflect.DeepEqual(got,want){t.Fatalf("library %#v\nupstream %#v",got,want)}})}
}
`

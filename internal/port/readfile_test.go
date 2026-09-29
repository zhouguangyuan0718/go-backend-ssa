package port

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestExplicitFileInputAndOwnedConstructorComments(t *testing.T) {
	source := `package p
 import host "os"
 type Node struct{A,B map[string]int}
 var sentinel=&Node{}
 // ConstructorDoc remains after a moved state initializer.
 func NewNode()*Node{return &Node{A:make(map[string]int),B:make(map[string]int)}}
 // Adjacent retains its own documentation.
 func Adjacent(){}
 func Read(name string)([]byte,error){return host.ReadFile(name)}
 func Callback()func(string)([]byte,error){return host.ReadFile}
 func Shadow()string{host:=struct{ReadFile func()string}{func()string{return "local"}};return host.ReadFile()}
 `
	repo, pin := fixtureRepo(t, map[string]string{"LICENSE": "fixture", "PATENTS": "fixture", "src/cmd/compile/internal/p/p.go": source})
	s, err := OpenSource(context.Background(), repo, pin)
	if err != nil {
		t.Fatal(err)
	}
	c := &ContextRule{Type: "Context", Constructor: "newContext", Selective: true, RedirectReadFile: true, Adapter: "adapter_context.go", Fields: map[string]string{"sentinel": "sentinel"}}
	plan := Plan{InspectRoots: []string{"cmd/compile/internal/p"}, Units: []Unit{{Source: "src/cmd/compile/internal/p/p.go", WholeFile: true, Context: c, Owner: &OwnerRule{Types: []string{"Node"}, Context: "Context", Field: "compilation"}}}}
	dir := t.TempDir()
	adapter := `package p;type Context struct{input string;sentinel *Node};func(c *Context)readFile(name string)([]byte,error){return []byte(c.input+name),nil}`
	writeTestFile(t, dir, "internal/p/adapter_context.go", adapter)
	r, err := Render(s, plan, dir)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(r.Files["internal/p/p.go"]), "// ConstructorDoc remains after a moved state initializer.\nfunc") {
		t.Fatal("state initialization lost adjacent documentation", string(r.Files["internal/p/p.go"]))
	}
	if !strings.Contains(string(r.Files["internal/p/p.go"]), "// Adjacent retains its own documentation.\nfunc Adjacent()") {
		t.Fatal("owned constructor moved adjacent documentation", string(r.Files["internal/p/p.go"]))
	}
	if err := Install(dir, r); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, dir, "go.mod", "module go-backend-ssa\n\ngo 1.27.1\n")
	writeTestFile(t, dir, "internal/p/adapter_test.go", `package p;import "testing"
 func TestInputs(t *testing.T){a,b:=newContext(),newContext();a.input="a";b.input="b";cb:=a.Callback();v,e:=cb("file");w,f:=b.Read("file");if string(v)!="afile"||string(w)!="bfile"||e!=nil||f!=nil||Shadow()!="local"||a.NewNode().compilation!=a{t.Fatal(string(v),string(w),e,f)}}`)
	cmd := exec.Command("go", "test", "-race", "./...")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOWORK=off", "GOTOOLCHAIN=go1.27.1")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	c.RedirectReadFile = false
	if _, err := Render(s, plan, dir); err == nil || !strings.Contains(err.Error(), "unclassified environment") {
		t.Fatalf("implicit filesystem accepted: %v", err)
	}
	c.RedirectReadFile = true
	s.cache[plan.Units[0].Source] = []byte(source + `func Bad(){host.WriteFile("x",nil,0600)}`)
	if _, err := Render(s, plan, dir); err == nil || !strings.Contains(err.Error(), "unclassified environment") {
		t.Fatalf("unreviewed file output accepted: %v", err)
	}
	s.cache[plan.Units[0].Source] = []byte(strings.Replace(source, "func Callback()func(string)([]byte,error){return host.ReadFile}", "", 1))
	writeTestFile(t, dir, "internal/p/adapter_context.go", strings.Replace(strings.Replace(adapter, "readFile(name string)", "readFile(input any)", 1), "c.input+name", "c.input+input.(string)", 1)+"\n")
	if _, err := Render(s, plan, dir); err == nil || !strings.Contains(err.Error(), "signature mismatch") {
		t.Fatalf("input adapter signature drift accepted: %v", err)
	}
}

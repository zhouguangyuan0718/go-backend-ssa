package port

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestTypedAdapterMethodBoundary(t *testing.T) {
	const source = `package p
 import("os";"fmt")
 var count int
 type Record struct {name string}
 func New(name string)*Record{return &Record{name:name}}
 func(r *Record)Emit()int{count++;n,_:=r.emit("%s:%d",r.name,count);return n}
 func(r *Record)emit(f string,a ...any)(int,error){os.Getenv("UNSUPPORTED_HOST_SETTING");return fmt.Printf(f,a...)}
 `
	const adapter = `package p
 import("fmt";"io")
 type Context struct{count int;Out io.Writer}
 func(r *Record)emit(format string,args ...any)(int,error){return fmt.Fprintf(r.owner.Out,format,args...)}
 `
	repo, pin := fixtureRepo(t, map[string]string{"LICENSE": "fixture", "PATENTS": "fixture", "src/cmd/compile/internal/p/p.go": source})
	s, err := OpenSource(context.Background(), repo, pin)
	if err != nil {
		t.Fatal(err)
	}
	plan := Plan{InspectRoots: []string{"cmd/compile/internal/p"}, Units: []Unit{{Source: "src/cmd/compile/internal/p/p.go", WholeFile: true, AdapterMethods: []string{"Record.emit"}, Owner: &OwnerRule{Types: []string{"Record"}, Context: "Context", Field: "owner"}, Context: &ContextRule{Type: "Context", Constructor: "NewContext", Selective: true, Adapter: "adapter_context.go", Fields: map[string]string{"count": "count"}}}}}
	dir := t.TempDir()
	writeTestFile(t, dir, "internal/p/adapter_context.go", adapter)
	r, err := Render(s, plan, dir)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(r.Files["internal/p/p.go"]), "Getenv") {
		t.Fatal("host method copied")
	}
	if err := Install(dir, r); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, dir, "go.mod", "module go-backend-ssa\n\ngo 1.27.1\n")
	writeTestFile(t, dir, "internal/p/adapter_test.go", `package p
 import("bytes";"testing")
 func TestBoundary(t *testing.T){a,b:=NewContext(),NewContext();var x,y bytes.Buffer;a.Out=&x;b.Out=&y;ra,rb:=a.New("a"),b.New("b");ra.Emit();rb.Emit();ra.Emit();if x.String()!="a:1a:2"||y.String()!="b:1"{t.Fatal(x.String(),y.String())}}
 `)
	cmd := exec.Command("go", "test", "-race", "./...")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOWORK=off", "GOTOOLCHAIN=go1.27.1")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	writeTestFile(t, dir, "internal/p/adapter_context.go", strings.Replace(adapter, "(r *Record)", "(r Record)", 1))
	if _, err := Render(s, plan, dir); err == nil || !strings.Contains(err.Error(), "signature mismatch") {
		t.Fatal("receiver drift accepted", err)
	}
	writeTestFile(t, dir, "internal/p/adapter_context.go", adapter)
	s.cache[plan.Units[0].Source] = []byte(strings.Replace(source, "f string,a ...any", "f string,a ...string", 1))
	if _, err := Render(s, plan, dir); err == nil {
		t.Fatal("signature drift accepted")
	}
	s.cache[plan.Units[0].Source] = []byte(source)
	plan.Units[0].AdapterMethods = []string{"Record.absent"}
	if _, err := Render(s, plan, dir); err == nil {
		t.Fatal("missing method accepted")
	}
}

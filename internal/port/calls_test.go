package port

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestExternalCallsUseSourceSignaturesAndObjectIdentity(t *testing.T) {
	host := `package host
func Report(format string,args ...any){unselectedProcessExit()}
`
	source := `package p
import renamed "cmd/compile/internal/host"
type Node struct{}
var n=1
func Fire(){renamed.Report("hello %d",n)}
func Callback()func(string,...any){return renamed.Report}
func Shadow()int{renamed:=struct{Report func()int}{func()int{return 7}};return renamed.Report()}
func(n *Node) Fire(){renamed.Report("method")}
// Adjacent remains attached to its own declaration.
func Adjacent(){renamed.Report("next")}
`
	repo, pin := fixtureRepo(t, map[string]string{"LICENSE": "fixture", "PATENTS": "fixture", "src/cmd/compile/internal/p/p.go": source, "src/cmd/compile/internal/host/host.go": host})
	s, err := OpenSource(context.Background(), repo, pin)
	if err != nil {
		t.Fatal(err)
	}
	call := CallRule{Source: "src/cmd/compile/internal/host/host.go", Function: "Report", Method: "report"}
	plan := Plan{InspectRoots: []string{"cmd/compile/internal/p"}, Units: []Unit{{Source: "src/cmd/compile/internal/p/p.go", Functions: []string{"Fire", "Callback", "Shadow", "Adjacent"}, Declarations: []string{"Node", "Node.Fire"}, Owner: &OwnerRule{Types: []string{"Node"}, Context: "Context", Field: "owner"}, Context: &ContextRule{Type: "Context", Constructor: "newContext", Adapter: "adapter_context.go", Fields: map[string]string{"n": "n"}}, Calls: []CallRule{call}}}}
	dir := t.TempDir()
	adapter := `package p;import "fmt";type Context struct{n int;messages []string};func(c *Context)report(format string,args ...any){c.messages=append(c.messages,fmt.Sprintf(format,args...))}`
	writeTestFile(t, dir, "internal/p/adapter_context.go", adapter)
	r, err := Render(s, plan, dir)
	if err != nil {
		t.Fatal(err)
	}
	if r.Ledger.Files[0].SignatureSources[call.Source] != hash([]byte(host)) {
		t.Fatal("signature source not recorded")
	}
	out := string(r.Files["internal/p/p.go"])
	if !strings.Contains(out, "// Adjacent remains attached to its own declaration.\nfunc") {
		t.Fatal("adjacent documentation moved into preceding body", out)
	}
	if err := Install(dir, r); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, dir, "go.mod", "module go-backend-ssa\n\ngo 1.27.1\n")
	writeTestFile(t, dir, "internal/p/adapter_test.go", `package p
import "testing"
func TestBindings(t *testing.T){a,b:=newContext(),newContext();a.Fire();cb:=a.Callback();cb("callback %d",2);(&Node{owner:b}).Fire();if len(a.messages)!=2||a.messages[0]!="hello 1"||a.messages[1]!="callback 2"||len(b.messages)!=1||a.Shadow()!=7{t.Fatal(a.messages,b.messages)}}
`)
	cmd := exec.Command("go", "test", "-race", "./...")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOWORK=off", "GOTOOLCHAIN=go1.27.1")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	// A more permissive adapter must still not conceal upstream signature drift.
	s.cache[call.Source] = []byte(strings.Replace(host, "func Report(format string,args ...any)", "func Report(format string,args ...any) int", 1))
	if _, err := Render(s, plan, dir); err == nil || !strings.Contains(err.Error(), "cannot use") && !strings.Contains(err.Error(), "signature mismatch") {
		t.Fatalf("signature drift accepted: %v", err)
	}
	s.cache[call.Source] = []byte(host)
	writeTestFile(t, dir, "internal/p/adapter_context.go", strings.Replace(adapter, "report(format string,args ...any)", "report(format string,args ...any) int", 1)+"\n")
	if _, err := Render(s, plan, dir); err == nil {
		t.Fatal("invalid adapter accepted")
	}
}

func TestRetainedNativeCall(t *testing.T) {
	const path = "src/cmd/compile/internal/p/p.go"
	const source = `package p
var count int
func native(n int)int{count++;return n*2}
func Run(n int)int{return native(n)+native(n+1)}
`
	const adapter = `package p
type Context struct{count,calls int;failure any}
func(c *Context)gate(n int)int{c.calls++;if n<0{panic(c.failure)};return c.native(n)}
`
	repo, pin := fixtureRepo(t, map[string]string{"LICENSE": "fixture", "PATENTS": "fixture", path: source})
	s, e := OpenSource(context.Background(), repo, pin)
	if e != nil {
		t.Fatal(e)
	}
	u := Unit{Source: path, Functions: []string{"native", "Run"}, Context: &ContextRule{Type: "Context", Constructor: "newContext", Selective: true, Adapter: "adapter_context.go", Fields: map[string]string{"count": "count"}}, Calls: []CallRule{{Source: path, Function: "native", Method: "gate", KeepDefinition: true}}}
	plan := Plan{InspectRoots: []string{"cmd/compile/internal/p"}, Units: []Unit{u}}
	dir := t.TempDir()
	writeTestFile(t, dir, "internal/p/adapter_context.go", adapter)
	r, e := Render(s, plan, dir)
	if e != nil {
		t.Fatal(e)
	}
	if e = Install(dir, r); e != nil {
		t.Fatal(e)
	}
	writeTestFile(t, dir, "go.mod", "module go-backend-ssa\n\ngo 1.27.1\n")
	writeTestFile(t, dir, "internal/p/check_test.go", `package p
import "testing"
func TestCalls(t *testing.T){c:=newContext();if n:=c.Run(3);n!=14||c.calls!=2||c.count!=2{t.Fatal(n,c.calls,c.count)};if c.native(1)!=2||c.calls!=2||c.count!=3{t.Fatal("native definition lost")};marker:=new(int);c.failure=marker;func(){defer func(){if recover()!=marker{t.Error("panic identity")}}();c.Run(-1)}();if c.count!=3||c.calls!=3{t.Fatal("failed entry reached native body")};other:=newContext();if other.Run(2)!=10||other.calls!=2||other.count!=2||c.count!=3{t.Fatal("shared state")}}
`)
	cmd := exec.Command("go", "test", "-race", "./...")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOWORK=off", "GOTOOLCHAIN=go1.27.1")
	if out, e := cmd.CombinedOutput(); e != nil {
		t.Fatalf("%v\n%s", e, out)
	}
	t.Run("missing definition", func(t *testing.T) {
		p := plan
		p.Units = append([]Unit(nil), plan.Units...)
		p.Units[0].Functions = []string{"Run"}
		if _, e := Render(s, p, dir); e == nil {
			t.Fatal("unselected retained body accepted")
		}
	})
	t.Run("unused binding", func(t *testing.T) {
		s.cache[path] = []byte(strings.Replace(source, "native(n)+native(n+1)", "n", 1))
		defer func() { s.cache[path] = []byte(source) }()
		if _, e := Render(s, plan, dir); e == nil {
			t.Fatal("unused retained call accepted")
		}
	})
	t.Run("wrong signature", func(t *testing.T) {
		writeTestFile(t, dir, "internal/p/adapter_context.go", strings.Replace(adapter, "gate(n int)", "gate(n int64)", 1))
		if _, e := Render(s, plan, dir); e == nil {
			t.Fatal("wrong wrapper signature accepted")
		}
	})
}

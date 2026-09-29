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

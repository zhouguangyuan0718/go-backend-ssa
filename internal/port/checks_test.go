package port

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestEntryChecksBeforeMutationAndSignatureDrift(t *testing.T) {
	source := `package p
type Node struct {Value int}
var calls int
func Make()*Node{return &Node{}}
func Store(items []*Node)int{calls++;return len(items)}
func (n *Node) Set(value int){n.Value=value}
`
	repo, pin := fixtureRepo(t, map[string]string{"LICENSE": "fixture", "PATENTS": "fixture", "src/cmd/compile/internal/p/p.go": source})
	s, err := OpenSource(context.Background(), repo, pin)
	if err != nil {
		t.Fatal(err)
	}
	plan := Plan{InspectRoots: []string{"cmd/compile/internal/p"}, Units: []Unit{{Source: "src/cmd/compile/internal/p/p.go", Functions: []string{"Make", "Store"}, Declarations: []string{"Node", "Node.Set"}, Owner: &OwnerRule{Types: []string{"Node"}, Context: "Context", Field: "owner"}, Context: &ContextRule{Type: "Context", Constructor: "newContext", Adapter: "adapter_context.go", Fields: map[string]string{"calls": "calls"}}, Checks: map[string]string{"Store": "checkItems", "Node.Set": "checkValue"}}}}
	dir := t.TempDir()
	adapter := `package p
type Context struct{calls,checks int}
func(c *Context)checkItems(items []*Node){for _,item:=range items{if item.Value<0{panic("bad input")}}}
func(c *Context)checkValue(value int){if value<0{panic("bad input")};c.checks++}
`
	writeTestFile(t, dir, "internal/p/adapter_context.go", adapter)
	r, err := Render(s, plan, dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := Install(dir, r); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, dir, "go.mod", "module go-backend-ssa\n\ngo 1.27.1\n")
	writeTestFile(t, dir, "internal/p/adapter_test.go", `package p
import "testing"
func TestChecks(t *testing.T){a,b:=newContext(),newContext();n:=a.Make();n.Value=-1
 fail:=func(f func()){defer func(){if recover()==nil{t.Error("check not called")}}();f()}
 fail(func(){a.Store([]*Node{n})});if a.calls!=0{t.Fatal("mutation before check")}
 n.Value=2;fail(func(){n.Set(-1)});if n.Value!=2{t.Fatal("receiver mutated before check")}
 cb:=n.Set;cb(7);expr:=(*Node).Set;expr(b.Make(),9)
 if a.checks!=1||b.checks!=1||n.Value!=7||a.Store([]*Node{n})!=1{t.Fatal("callback lost validation/context")}
}
`)
	cmd := exec.Command("go", "test", "-race", "./...")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOWORK=off", "GOTOOLCHAIN=go1.27.1")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	// An adapter accepting any would compile at call sites but must fail the
	// explicit signature check, rather than silently weakening the contract.
	changed := strings.Replace(adapter, "func(c *Context)checkValue(value int){if value<0{panic(\"bad input\")};c.checks++}", "func(c *Context)checkValue(value any){}", 1)
	writeTestFile(t, dir, "internal/p/adapter_context.go", changed)
	if _, err := Render(s, plan, dir); err == nil || !strings.Contains(err.Error(), "validator signature mismatch") {
		t.Fatalf("permissive validator accepted: %v", err)
	}
}

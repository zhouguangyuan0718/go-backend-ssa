package port

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestNullableOwnedInputsPreserveNilAndRejectForeign(t *testing.T) {
	const source = `package p
 type Node struct{value int}
 var calls int
 func New()*Node{return &Node{value:7}}
 func Maybe(n *Node)int{calls++;if n==nil{return -1};return n.value}
 func Required(n *Node)int{calls++;return n.value}
 func text(n *Node)string{if n==nil{return "<nil>"};calls++;return "node"}
 func(n *Node)Text()string{return text(n)}
 `
	repo, pin := fixtureRepo(t, map[string]string{"LICENSE": "fixture", "PATENTS": "fixture", "src/cmd/compile/internal/p/p.go": source})
	s, err := OpenSource(context.Background(), repo, pin)
	if err != nil {
		t.Fatal(err)
	}
	plan := Plan{InspectRoots: []string{"cmd/compile/internal/p"}, Units: []Unit{{Source: "src/cmd/compile/internal/p/p.go", Functions: []string{"New", "Maybe", "Required", "text"}, Declarations: []string{"Node", "Node.Text"}, Owner: &OwnerRule{Types: []string{"Node"}, Context: "Context", Field: "owner"}, Context: &ContextRule{Type: "Context", Constructor: "newContext", Adapter: "adapter_context.go", Fields: map[string]string{"calls": "calls"}}, Nullable: map[string][]string{"Maybe": {"n"}, "text": {"n"}, "Node.Text": {"n"}}}}}
	dir := t.TempDir()
	writeTestFile(t, dir, "internal/p/adapter_context.go", `package p;type Context struct{calls int}`)
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
 func TestOwnership(t *testing.T){a,b:=newContext(),newContext();if a.Maybe(nil)!=-1||a.Maybe(a.New())!=7||(*Node)(nil).Text()!="<nil>"{t.Fatal("nil semantics")};old:=a.calls;fail:=func(f func()){defer func(){if recover()==nil{t.Error("accepted invalid argument")}}();f()};fail(func(){a.Maybe(b.New())});fail(func(){a.Required(nil)});fail(func(){a.Maybe(&Node{})});if a.calls!=old||b.calls!=0{t.Fatal("mutated before validation")}}
 `)
	cmd := exec.Command("go", "test", "-race", "./...")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOWORK=off", "GOTOOLCHAIN=go1.27.1")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	plan.Units[0].Nullable["Maybe"] = []string{"renamed"}
	if _, err := Render(s, plan, dir); err == nil || !strings.Contains(err.Error(), "nullable parameter not found") {
		t.Fatalf("stale nullable rule accepted: %v", err)
	}
}

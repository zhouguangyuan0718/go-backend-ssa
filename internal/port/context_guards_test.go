package port

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestContextAfterPurePointerGuards(t *testing.T) {
	source := `package p
 type Node struct{value int}
 var calls int
 func New(value int)*Node{return &Node{value:value}}
 func cmpForNe(less bool)int{if less{return -1};return 1}
 func(n *Node)Compare(x *Node)int{
  if n==x{return 0}
  if n==nil{return -1}
  if x==nil{return 1}
  calls++
  return cmpForNe(n.value<x.value)
 }
 `
	repo, pin := fixtureRepo(t, map[string]string{"LICENSE": "fixture", "PATENTS": "fixture", "src/cmd/compile/internal/p/p.go": source})
	s, err := OpenSource(context.Background(), repo, pin)
	if err != nil {
		t.Fatal(err)
	}
	unit := Unit{Source: "src/cmd/compile/internal/p/p.go", WholeFile: true, Context: &ContextRule{Type: "Context", Constructor: "newContext", Adapter: "adapter_context.go", Fields: map[string]string{"calls": "calls"}}, Owner: &OwnerRule{Types: []string{"Node"}, Context: "Context", Field: "owner"}, StatelessFunctions: []string{"cmpForNe"}, ContextAfterGuards: map[string]int{"Node.Compare": 3}}
	plan := Plan{InspectRoots: []string{"cmd/compile/internal/p"}, Units: []Unit{unit}}
	dir := t.TempDir()
	writeTestFile(t, dir, "internal/p/adapter_context.go", "package p;type Context struct{calls int}")
	rendered, err := Render(s, plan, dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := Install(dir, rendered); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, dir, "go.mod", "module go-backend-ssa\n\ngo 1.27.1\n")
	writeTestFile(t, dir, "internal/p/check_test.go", `package p
 import "testing"
 func TestGuards(t *testing.T){a,b:=newContext(),newContext();x,y,z:=a.New(1),a.New(2),b.New(3);var nilNode *Node
 if nilNode.Compare(nil)!=0||nilNode.Compare(x)!=-1||x.Compare(nil)!=1||x.Compare(x)!=0||a.calls!=0||cmpForNe(true)!=-1{t.Fatal("early return or stateless helper changed")}
 if x.Compare(y)!=-1||a.calls!=1{t.Fatal("owned comparison")}
 defer func(){if recover()==nil||a.calls!=1||b.calls!=0{t.Fatal("foreign comparison reached state")}}();x.Compare(z)
 }
 `)
	cmd := exec.Command("go", "test", "-race", "./...")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOWORK=off", "GOTOOLCHAIN=go1.27.1")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	for name, replacement := range map[string]string{
		"read through parameter": "if n==x{return n.value}",
		"call in condition":      "if n==New(1){return 0}",
		"mutation in branch":     "if n==x{calls++;return 0}",
		"else branch":            "if n==x{return 0}else{return 1}",
	} {
		t.Run(name, func(t *testing.T) {
			s.cache[unit.Source] = []byte(strings.Replace(source, "if n==x{return 0}", replacement, 1))
			if _, err := Render(s, plan, dir); err == nil || !strings.Contains(err.Error(), "context guard prefix") {
				t.Fatalf("invalid prefix accepted: %v", err)
			}
		})
	}
	t.Run("helper gains state", func(t *testing.T) {
		s.cache[unit.Source] = []byte(strings.Replace(source, "if less{return -1}", "calls++;if less{return -1}", 1))
		if _, err := Render(s, plan, dir); err == nil || !strings.Contains(err.Error(), "stateless function uses compilation state") {
			t.Fatalf("stateful helper accepted: %v", err)
		}
	})
}

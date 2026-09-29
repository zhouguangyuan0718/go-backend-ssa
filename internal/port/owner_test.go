package port

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestOwnerInjectionAndReceiverCallbacks(t *testing.T) {
	source := `package p
type Node struct{ N int }
var root=new(Node)
var calls int
const (First=iota; Second; Third)
func Make() *Node { calls++; return new(Node) }
func (p *Node) Root() *Node {calls++;return root}
func (p *Node) IsRoot()bool{if p==nil{return false};return p==root}
func (p *Node) Clone() *Node {return &Node{N:p.N}}
func (p *Node) Closure() func() *Node {return func()*Node{return root}}
func Local() int {type Node struct{N int};x:=new(Node);return x.N}
`
	repo, pin := fixtureRepo(t, map[string]string{"LICENSE": "fixture", "PATENTS": "fixture", "src/cmd/compile/internal/p/p.go": source})
	s, err := OpenSource(context.Background(), repo, pin)
	if err != nil {
		t.Fatal(err)
	}
	plan := Plan{InspectRoots: []string{"cmd/compile/internal/p"}, Units: []Unit{{Source: "src/cmd/compile/internal/p/p.go", Functions: []string{"Make", "Local"}, Declarations: []string{"Node", "Node.Root", "Node.IsRoot", "Node.Clone", "Node.Closure", "Third"}, Owner: &OwnerRule{Types: []string{"Node"}, Context: "Context", Field: "owner"}, Context: &ContextRule{Type: "Context", Constructor: "newContext", Fields: map[string]string{"root": "root", "calls": "calls"}, Adapter: "adapter_context.go"}}}}
	dir := t.TempDir()
	writeTestFile(t, dir, "internal/p/adapter_context.go", "package p; type Context struct{root *Node; calls int}")
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
func TestOwnership(t *testing.T){
 a,b:=newContext(),newContext()
 if (*Node)(nil).IsRoot() || !a.root.IsRoot() {t.Fatal("nil receiver semantics lost")}
 if a.root.owner!=a || b.root.owner!=b || a.root==b.root {t.Fatal("initializer root ownership lost")}
 p,q:=a.Make(),b.Make();p.N=17
 if p.owner!=a || q.owner!=b || a.Local()!=0 || Third!=2 {t.Fatal("allocation, shadow or iota changed")}
 clone:=p.Clone();if clone.owner!=a || clone.N!=17 {t.Fatal("allocation-only method lost owner")}
 callback:=p.Root;expr:=(*Node).Root;closure:=p.Closure()
 if callback()!=a.root || expr(q)!=b.root || closure()!=a.root || a.calls!=2 || b.calls!=2 {t.Fatal("receiver/callback context changed")}
}
`)
	cmd := exec.Command("go", "test", "-race", "./...")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOWORK=off", "GOTOOLCHAIN=go1.27.1")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	for _, tc := range []struct{ name, source, want string }{
		{"unkeyed", strings.Replace(source, "return &Node{N:p.N}", "return &Node{p.N}", 1), "unkeyed"},
		{"missing owner", source, "requires an owner rule"},
		{"zero local", strings.Replace(source, "return new(Node)", "var n Node; return &n", 1), "zero declaration"},
		{"zero local in receiver", strings.Replace(source, "return &Node{N:p.N}", "var n Node; return &n", 1), "zero declaration"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			u := plan.Units[0]
			if tc.name == "missing owner" {
				u.Owner = nil
			}
			subplan := Plan{InspectRoots: plan.InspectRoots, Units: []Unit{u}}
			saved := s.cache[u.Source]
			s.cache[u.Source] = []byte(tc.source)
			defer func() { s.cache[u.Source] = saved }()
			if _, err := Render(s, subplan, dir); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("want %q, got %v", tc.want, err)
			}
		})
	}
}

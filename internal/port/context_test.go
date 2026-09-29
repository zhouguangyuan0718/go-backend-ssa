package port

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestCrossFileContextInitialization(t *testing.T) {
	a := `package p
import x "strings"
type Node struct { N int }
var first=2
var last=FromOther()
var mark=x.ToUpper("a")
func init(){ first+=last; record+=mark }
func (n *Node) Current()*Node{return root}
func (n Node) Clone()*Node{return &Node{N:n.N}}
func (n *Node) Empty()bool{return n==nil}
func Accept(n *Node)*Node{return n}
`
	b := `package p
import x "strconv"
var second=first+3
var label=x.Itoa(last)
var root=Make()
var record string
func FromOther()int{return second+1}
func Make()*Node{return &Node{N:last}}
func Callback()func()int{return FromOther}
func AcceptMany(nodes []*Node)int{return len(nodes)}
func init(){first++;record+="B";defer func(){record+="D"}();return}
func init(){record+="C"}
`
	repo, pin := fixtureRepo(t, map[string]string{"LICENSE": "fixture", "PATENTS": "fixture", "src/cmd/compile/internal/p/a.go": a, "src/cmd/compile/internal/p/b.go": b})
	source, err := OpenSource(context.Background(), repo, pin)
	if err != nil {
		t.Fatal(err)
	}
	c := func(fields map[string]string) *ContextRule {
		return &ContextRule{Type: "Context", Constructor: "newContext", Adapter: "adapter_context.go", Fields: fields, Init: true}
	}
	// Reverse plan order deliberately; Go file initialization still follows names.
	plan := Plan{InspectRoots: []string{"cmd/compile/internal/p"}, Units: []Unit{
		{Source: "src/cmd/compile/internal/p/b.go", Functions: []string{"FromOther", "Make", "Callback", "AcceptMany"}, Context: c(map[string]string{"second": "second", "label": "label", "root": "root", "record": "record"})},
		{Source: "src/cmd/compile/internal/p/a.go", Functions: []string{"Accept"}, Declarations: []string{"Node", "Node.Current", "Node.Clone", "Node.Empty"}, Owner: &OwnerRule{Types: []string{"Node"}, Context: "Context", Field: "owner"}, Context: c(map[string]string{"first": "first", "last": "last", "mark": "mark"})},
	}}
	dir := t.TempDir()
	writeTestFile(t, dir, "internal/p/adapter_context.go", `package p;type Context struct{first,last,second int;mark,label,record string;root *Node}`)
	rendered, err := Render(source, plan, dir)
	if err != nil {
		t.Fatal(err)
	}
	if err = Install(dir, rendered); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, dir, "go.mod", "module go-backend-ssa\n\ngo 1.27.1\n")
	writeTestFile(t, dir, "internal/p/adapter_test.go", `package p
import "testing"
func TestBehavior(t *testing.T){
 a,b:=newContext(),newContext()
 if a.first!=9 || a.last!=6 || a.label!="6" || a.record!="ABDC" || a.root.N!=6 {t.Fatalf("initialization reordered: %+v",a)}
 if a.root.owner!=a || b.root.owner!=b || a.root==b.root {t.Fatal("cross-file allocation lost owner")}
 method:=a.root.Current;expr:=(*Node).Current;cb:=a.Callback()
 if method()!=a.root || expr(b.root)!=b.root || cb()!=6 {t.Fatal("cross-file callbacks lost context")}
 clone:=a.root.Clone()
 if clone.owner!=a || clone.N!=6 || !(*Node)(nil).Empty() {t.Fatal("receiver behavior changed")}
 if a.Accept(a.root)!=a.root || a.AcceptMany([]*Node{a.root,clone})!=2 {t.Fatal("valid arguments rejected")}
 for _, f:=range []func(){func(){a.Accept(b.root)},func(){a.Accept(nil)},func(){a.AcceptMany([]*Node{b.root})},func(){a.AcceptMany([]*Node{nil})}} {
  func(){defer func(){if recover()==nil{t.Error("foreign/nil graph accepted")}}();f()}()
 }
 a.second=40
 if cb()!=41 || b.Callback()()!=6 || b.record!="ABDC" {t.Fatal("shared state")}
}
`)
	cmd := exec.Command("go", "test", "-race", "./...")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOWORK=off", "GOTOOLCHAIN=go1.27.1")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	// Fail closed if the reviewed init grows a dependency outside the slice.
	saved := source.cache[plan.Units[0].Source]
	source.cache[plan.Units[0].Source] = []byte(b + "\nfunc init(){unselected()}\n")
	defer func() { source.cache[plan.Units[0].Source] = saved }()
	if _, err := Render(source, plan, dir); err == nil || !strings.Contains(err.Error(), "undefined: unselected") {
		t.Fatalf("new init dependency accepted: %v", err)
	}
}

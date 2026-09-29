package port

import (
	"context"
	"os"
	"os/exec"
	"testing"
)

func TestOwnedZeroDeclarations(t *testing.T) {
	const source = `package p
 type Context struct{}
 func NewContext()*Context{return &Context{}}
 type Node struct{N int}
 var root Node
 var number=4
 var snapshot=root.Read()
 func(n *Node)Read()int{return number+n.N}
 func(n *Node)Zero()*Node{var child Node;return &child}
 func Locals()(*Node,*Node){var x,y Node;x.N=3;return &x,&y}
 `
	repo, pin := fixtureRepo(t, map[string]string{"LICENSE": "fixture", "PATENTS": "fixture", "src/cmd/compile/internal/p/p.go": source})
	s, err := OpenSource(context.Background(), repo, pin)
	if err != nil {
		t.Fatal(err)
	}
	plan := Plan{InspectRoots: []string{"cmd/compile/internal/p"}, Units: []Unit{{Source: "src/cmd/compile/internal/p/p.go", WholeFile: true, Owner: &OwnerRule{Types: []string{"Node"}, Context: "Context", Field: "owner"}, Context: &ContextRule{Existing: true, Selective: true, Type: "Context", Constructor: "NewContext", Fields: map[string]string{"root": "root", "number": "number", "snapshot": "snapshot"}}}}}
	dir := t.TempDir()
	r, err := Render(s, plan, dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := Install(dir, r); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, dir, "go.mod", "module go-backend-ssa\n\ngo 1.27.1\n")
	writeTestFile(t, dir, "internal/p/adapter_test.go", `package p
 import("testing";"sync")
 func TestZeros(t *testing.T){run:=func(){a,b:=NewContext(),NewContext();x,y:=a.Locals();z:=x.Zero();if a.root.owner!=a||b.root.owner!=b||a.snapshot!=4||x==y||x.N!=3||y.N!=0||x.owner!=a||y.owner!=a||z.owner!=a||z.N!=0{t.Error("zero state ownership or initialization")}};run();var wg sync.WaitGroup;for range 16{wg.Go(run)};wg.Wait()}
 `)
	cmd := exec.Command("go", "test", "-race", "./...")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOWORK=off", "GOTOOLCHAIN=go1.27.1")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
}

package port

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestExplicitPublicValueMethodContext(t *testing.T) {
	const input = `package p
 var names=[1]string{"native"}
 const Last=Opcode(len(names)-1)
 const Capacity=cap(names)
 type Opcode int
 func(o Opcode)String()string{return names[o]}
 func Bound(o Opcode)func()string{return o.String}
 func Expr(o Opcode)string{return Opcode.String(o)}
 func Direct(o Opcode)string{return o.String()}
 `
	repo, pin := fixtureRepo(t, map[string]string{"LICENSE": "fixture", "PATENTS": "fixture", "src/cmd/compile/internal/p/p.go": input})
	source, err := OpenSource(context.Background(), repo, pin)
	if err != nil {
		t.Fatal(err)
	}
	rule := Unit{Source: "src/cmd/compile/internal/p/p.go", WholeFile: true, Context: &ContextRule{Type: "Context", Constructor: "NewContext", Selective: true, Adapter: "adapter_context.go", Fields: map[string]string{"names": "Names"}}, ContextParamExports: []string{"Opcode.String"}}
	plan := Plan{InspectRoots: []string{"cmd/compile/internal/p"}, Units: []Unit{rule}}
	dir := t.TempDir()
	writeTestFile(t, dir, "internal/p/adapter_context.go", "package p;type Context struct {Names [1]string}")
	rendered, err := Render(source, plan, dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := Install(dir, rendered); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, dir, "go.mod", "module go-backend-ssa\n\ngo 1.27.1\n")
	writeTestFile(t, dir, "internal/p/adapter_test.go", `package p
 import "testing"
 func TestContext(t *testing.T){a,b:=NewContext(),NewContext();if Last!=0||Capacity!=1{t.Fatal("array constant changed")};a.Names[0]="a";b.Names[0]="b";bound:=a.Bound(0);if bound()!="a"||b.Direct(0)!="b"||a.Expr(0)!="a"||Opcode(0).String(b)!="b"{t.Fatal("context not retained")}}
 `)
	cmd := exec.Command("go", "test", "-race", "./...")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOWORK=off", "GOTOOLCHAIN=go1.27.1")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	for _, names := range [][]string{{"Opcode.*"}, {"Opcode.missing"}, {"private.String"}, {"Opcode.String", "Opcode.String"}} {
		plan.Units[0].ContextParamExports = names
		if err := plan.Validate(); err == nil {
			t.Fatalf("invalid exports accepted: %v", names)
		}
	}
	plan.Units[0].ContextParamExports = []string{"Opcode.Missing"}
	if _, err := Render(source, plan, dir); err == nil || !strings.Contains(err.Error(), "not selected") {
		t.Fatalf("missing method accepted: %v", err)
	}
}

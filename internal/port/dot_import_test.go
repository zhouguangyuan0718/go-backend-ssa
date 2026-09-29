package port

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestDotImportsKeepObjectIdentityAndRemoveUnused(t *testing.T) {
	const dependency = `package code;type Code int;const Err Code=7;func Twice(n int)int{return n*2}`
	const source = `package p
 import . "cmd/compile/internal/code"
 var seed Code=Err
 func Value()Code{return seed}
 func Shadow()int{Err:=9;return Err}
 func Bound()func(int)int{return Twice}
 `
	repo, pin := fixtureRepo(t, map[string]string{"LICENSE": "fixture", "PATENTS": "fixture", "src/cmd/compile/internal/code/code.go": dependency, "src/cmd/compile/internal/p/p.go": source})
	s, err := OpenSource(context.Background(), repo, pin)
	if err != nil {
		t.Fatal(err)
	}
	plan := Plan{InspectRoots: []string{"cmd/compile/internal/p"}, Units: []Unit{
		{Source: "src/cmd/compile/internal/code/code.go", WholeFile: true},
		{Source: "src/cmd/compile/internal/p/p.go", Functions: []string{"Value", "Shadow", "Bound"}, Context: &ContextRule{Type: "Context", Constructor: "newContext", Adapter: "adapter_context.go", Fields: map[string]string{"seed": "seed"}}},
	}}
	dir := t.TempDir()
	writeTestFile(t, dir, "internal/p/adapter_context.go", `package p;import "go-backend-ssa/internal/code";type Context struct{seed code.Code}`)
	r, err := Render(s, plan, dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := Install(dir, r); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, dir, "go.mod", "module go-backend-ssa\n\ngo 1.27.1\n")
	writeTestFile(t, dir, "internal/p/adapter_test.go", `package p;import "testing";func TestValues(t *testing.T){c:=newContext();if c.Value()!=7||c.Shadow()!=9||c.Bound()(6)!=12{t.Fatal("dot-import binding")}}`)
	cmd := exec.Command("go", "test", "-race", "./...")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOWORK=off", "GOTOOLCHAIN=go1.27.1")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	// Declaration selection leaves an unused dot import in the source slice.
	// Resolve first, then remove it instead of hiding an import in final output.
	subset := plan
	subset.Units = append([]Unit(nil), plan.Units...)
	subset.Units[1] = Unit{Source: plan.Units[1].Source, Functions: []string{"Shadow"}}
	fresh := t.TempDir()
	result, err := Render(s, subset, fresh)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(result.Files["internal/p/p.go"]), "import") {
		t.Fatal("unused dot import retained")
	}
	// Ambient functions stay visible to the output audit even when unqualified.
	s.cache[plan.Units[1].Source] = []byte(`package p;import . "os";func Shadow(){Exit(1)}`)
	if _, err := Render(s, subset, fresh); err == nil || !strings.Contains(err.Error(), "environment dependency") {
		t.Fatalf("dot import concealed process exit: %v", err)
	}
}

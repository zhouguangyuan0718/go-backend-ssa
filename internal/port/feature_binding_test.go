package port

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestInstanceFeatureConstantAndPrivateValueMethods(t *testing.T) {
	const input = `package p
 import "internal/features"
 var cache=map[summary]int{}
 type summary [2]byte
 func(s summary)Encode()int{if !features.Enabled{return -1};cache[s]++;return cache[s]}
 func Run()int{s:=summary{1,2};bound:=s.Encode;return bound()}
 func Expr()int{return summary.Encode(summary{1,2})}
 `
	repo, pin := fixtureRepo(t, map[string]string{"LICENSE": "fixture", "PATENTS": "fixture", "src/cmd/compile/internal/p/p.go": input, "src/internal/features/feature.go": "package features; const Enabled = false"})
	source, err := OpenSource(context.Background(), repo, pin)
	if err != nil {
		t.Fatal(err)
	}
	rule := Unit{Source: "src/cmd/compile/internal/p/p.go", WholeFile: true, ContextParams: []string{"summary.Encode"}, Context: &ContextRule{Type: "Context", Constructor: "NewContext", Selective: true, Adapter: "adapter_context.go", Fields: map[string]string{"cache": "cache"}}, State: []StateRule{{Source: "src/internal/features/feature.go", Variable: "Enabled", Field: "Enabled"}}}
	plan := Plan{InspectRoots: []string{"cmd/compile/internal/p"}, Units: []Unit{rule}}
	dir := t.TempDir()
	adapter := `package p;type Context struct{Enabled bool;cache map[summary]int}`
	writeTestFile(t, dir, "internal/p/adapter_context.go", adapter)
	rendered, err := Render(source, plan, dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := Install(dir, rendered); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, dir, "go.mod", "module go-backend-ssa\n\ngo 1.27.1\n")
	writeTestFile(t, dir, "internal/p/adapter_test.go", `package p
 import("testing";"sync")
 func TestFeatures(t *testing.T){a,b:=NewContext(),NewContext();a.Enabled=true;if a.Run()!=1||a.Expr()!=2||b.Run()!=-1||len(b.cache)!=0{t.Fatal("feature or cache shared")};var wg sync.WaitGroup;for range 8{wg.Go(func(){c:=NewContext();c.Enabled=true;if c.Run()!=1{t.Error("shared summary cache")}})};wg.Wait()}
 `)
	cmd := exec.Command("go", "test", "-race", "./...")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOTOOLCHAIN=go1.27.1", "GOWORK=off")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	plan.Units[0].ContextParams = []string{"Summary.Encode"}
	if err := plan.Validate(); err == nil {
		t.Fatal("public receiver signature change accepted")
	}
	plan.Units[0] = rule
	source.cache[rule.State[0].Source] = []byte("package features;const Enabled = 3")
	if _, err := Render(source, plan, dir); err == nil || !strings.Contains(err.Error(), "boolean") {
		t.Fatalf("non-boolean feature accepted: %v", err)
	}
}

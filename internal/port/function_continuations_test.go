package port

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestNativeFunctionContinuation(t *testing.T) {
	const sourcePath = "src/cmd/compile/internal/p/main.go"
	const depPath = "src/cmd/internal/dependency/data.go"
	const dependency = `package dependency
type Data struct{N int}
func Step(d *Data){d.N+=2}
`
	const source = `package p
import "cmd/internal/dependency"
type state struct{d *dependency.Data}
var done int
func finish(){done++}
func Main()*dependency.Data {
 s:=state{d:&dependency.Data{N:7}}
 defer finish()
 dependency.Step(s.d)
 s.d.N*=3
 return s.d
}
`
	const adapter = `package p
import "go-backend-ssa/internal/cmd/dependency"
type Context struct{done int;hold bool;saved func()*dependency.Data;built *dependency.Data}
func(c *Context)boundary(s *state,tail func()*dependency.Data)*dependency.Data{if !c.hold{return tail()};c.saved=tail;c.built=s.d;return s.d}
`
	plan := Plan{InspectRoots: []string{"cmd/compile/internal/p"}, Units: []Unit{
		{Source: depPath, WholeFile: true},
		{Source: sourcePath, WholeFile: true, Context: &ContextRule{Type: "Context", Constructor: "newContext", Selective: true, Adapter: "adapter_context.go", Fields: map[string]string{"done": "done"}}, FunctionContinuations: map[string]FunctionContinuationRule{"Main": {Before: SourceCallAnchor{Package: "cmd/internal/dependency", Member: "Step"}, Method: "boundary", State: "s"}}},
	}}
	render := func(t *testing.T, code string) (*Rendered, string, error) {
		t.Helper()
		repo, pin := fixtureRepo(t, map[string]string{"LICENSE": "fixture", "PATENTS": "fixture", sourcePath: code, depPath: dependency})
		input, err := OpenSource(context.Background(), repo, pin)
		if err != nil {
			t.Fatal(err)
		}
		root := t.TempDir()
		writeTestFile(t, root, "internal/p/adapter_context.go", adapter)
		result, err := Render(input, plan, root)
		return result, root, err
	}
	result, root, err := render(t, source)
	if err != nil {
		t.Fatal(err)
	}
	if err = Install(root, result); err != nil {
		t.Fatal(err)
	}
	seen := false
	for _, file := range result.Ledger.Files {
		if file.Output == "internal/p/main.go" {
			seen = strings.Contains(strings.Join(file.Declarations, ";"), "function-continuation:Main:cmd/internal/dependency.Step:boundary:s")
		}
	}
	if !seen {
		t.Fatal("missing continuation provenance")
	}
	writeTestFile(t, root, "go.mod", "module go-backend-ssa\n\ngo 1.27.1\n")
	writeTestFile(t, root, "internal/p/check_test.go", `package p
import("testing";"fmt")
func TestContinuation(t *testing.T){for i:=range 8{t.Run(fmt.Sprint(i),func(t *testing.T){t.Parallel();native:=newContext();n:=native.Main();if n.N!=27||native.done!=1{t.Fatal(n,native.done)};held:=newContext();held.hold=true;b:=held.Main();if b.N!=7||held.done!=1||b!=held.built{t.Fatal("prefix did not stop")};other:=newContext();other.Main();resumed:=held.saved();if resumed!=b||b.N!=27||held.done!=1||n.N!=27{t.Fatal("native suffix or ownership changed")}})}}
`)
	cmd := exec.Command("go", "test", "-race", "./...")
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "GOTOOLCHAIN=go1.27.1", "GOWORK=off")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	for name, code := range map[string]string{
		"missing":      strings.Replace(source, "dependency.Step(s.d)", "s.d.N+=2", 1),
		"duplicate":    strings.Replace(source, "dependency.Step(s.d)", "dependency.Step(s.d);dependency.Step(s.d)", 1),
		"shadow":       strings.Replace(source, "dependency.Step(s.d)", "if true{s:=state{};_=s};dependency.Step(s.d)", 1),
		"named-result": strings.Replace(source, "Main()*dependency.Data", "Main()(out *dependency.Data)", 1),
		"branch":       strings.Replace(source, "dependency.Step(s.d)", "goto next;next:dependency.Step(s.d)", 1),
	} {
		t.Run(name, func(t *testing.T) {
			if _, _, err := render(t, code); err == nil {
				t.Fatal("unsafe continuation accepted")
			}
		})
	}
}

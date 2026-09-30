package port

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestNativeFunctionRegion(t *testing.T) {
	const sourcePath = "src/cmd/compile/internal/p/main.go"
	const depPath = "src/cmd/internal/dependency/data.go"
	const dep = `package dependency
type Data struct{N int}
func Make()*Data{panic("unselected initializer executed")}
func Start(){}
func Stop(){}
`
	const source = `package p
import("os";"cmd/internal/dependency")
var total int
func Main(host func()){
 x:=dependency.Make()
 var y *dependency.Data
 host()
 os.Exit(2)
 dependency.Start()
 for n:=0;n<x.N;n++ {y.N+=n;total++}
 dependency.Stop()
 host()
}
`
	const adapter = `package p
type Context struct{total int}
`
	repo, pin := fixtureRepo(t, map[string]string{"LICENSE": "fixture", "PATENTS": "fixture", sourcePath: source, depPath: dep})
	s, e := OpenSource(context.Background(), repo, pin)
	if e != nil {
		t.Fatal(e)
	}
	unit := Unit{Source: sourcePath, Functions: []string{"Main"}, Context: &ContextRule{Type: "Context", Constructor: "newContext", Selective: true, Adapter: "adapter_context.go", Fields: map[string]string{"total": "total"}}, ContextMethods: map[string]string{"Main": "run"}, FunctionRegions: map[string]FunctionRegionRule{"Main": {First: SourceCallAnchor{Package: "cmd/internal/dependency", Member: "Start"}, Last: SourceCallAnchor{Package: "cmd/internal/dependency", Member: "Stop"}, Inputs: []string{"x", "y"}}}}
	plan := Plan{InspectRoots: []string{"cmd/compile/internal/p"}, Units: []Unit{{Source: depPath, WholeFile: true}, unit}}
	dir := t.TempDir()
	writeTestFile(t, dir, "internal/p/adapter_context.go", adapter)
	r, e := Render(s, plan, dir)
	if e != nil {
		t.Fatal(e)
	}
	if e = Install(dir, r); e != nil {
		t.Fatal(e)
	}
	data, e := os.ReadFile(dir + "/internal/p/main.go")
	if e != nil {
		t.Fatal(e)
	}
	if strings.Contains(string(data), "os.Exit") || strings.Contains(string(data), "host()") || strings.Contains(string(data), "dependency.Make") || !strings.Contains(string(data), "func (syncgoCtx *Context) run(x *dependency.Data, y *dependency.Data)") {
		t.Fatalf("region body/signature lost: %s", data)
	}
	var proven bool
	for _, entry := range r.Ledger.Files {
		if entry.Output == "internal/p/main.go" {
			proven = entry.SignatureSources[depPath] == hash([]byte(dep))
			if !strings.Contains(strings.Join(entry.Declarations, ";"), "function-region:Main:") {
				t.Fatal("missing region provenance")
			}
		}
	}
	if !proven {
		t.Fatal("missing initializer signature provenance")
	}
	writeTestFile(t, dir, "go.mod", "module go-backend-ssa\n\ngo 1.27.1\n")
	writeTestFile(t, dir, "internal/p/check_test.go", `package p
import("sync";"testing";"go-backend-ssa/internal/cmd/dependency")
func TestRegion(t *testing.T){var w sync.WaitGroup;for i:=range 8{w.Go(func(){c:=newContext();x,y:=&dependency.Data{N:i+2},&dependency.Data{};c.run(x,y);saved:=y.N;next:=newContext();next.run(&dependency.Data{N:i+3},&dependency.Data{});if c.total!=i+2||y.N!=(i+1)*(i+2)/2||y.N!=saved||next.total!=i+3{t.Error("region semantics",c.total,y.N,next.total)}})};w.Wait()}
`)
	cmd := exec.Command("go", "test", "-race", "./...")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOWORK=off", "GOTOOLCHAIN=go1.27.1")
	if out, e := cmd.CombinedOutput(); e != nil {
		t.Fatalf("%v\n%s", e, out)
	}
	for name, changed := range map[string]string{
		"missing-anchor":   strings.Replace(source, "dependency.Stop()", "host()", 1),
		"ambiguous-anchor": strings.Replace(source, "dependency.Start()", "dependency.Start();dependency.Start()", 1),
		"unbound-capture":  strings.Replace(strings.Replace(source, "host()\n os.Exit", "extra:=7;host()\n os.Exit", 1), "y.N+=n", "y.N+=n+extra", 1),
		"input-assignment": strings.Replace(source, "dependency.Start()", "dependency.Start();x=y", 1),
		"input-address":    strings.Replace(source, "dependency.Start()", "dependency.Start();_=&x", 1),
		"return":           strings.Replace(source, "dependency.Start()", "dependency.Start();if x.N<0{return}", 1),
		"defer":            strings.Replace(source, "dependency.Start()", "dependency.Start();defer dependency.Stop()", 1),
		"recover":          strings.Replace(source, "dependency.Start()", "dependency.Start();_ = recover()", 1),
		"closure":          strings.Replace(source, "dependency.Start()", "dependency.Start();func(){}()", 1),
		"label":            strings.Replace(strings.Replace(source, "for n:=0", "loop: for n:=0", 1), "y.N+=n", "if n>3{break loop};y.N+=n", 1),
		"escaping-local":   strings.Replace(strings.Replace(source, "dependency.Start()", "dependency.Start();z:=1;_ = z", 1), "dependency.Stop()", "dependency.Stop();_ = z", 1),
		"shadowed-anchor":  strings.Replace(source, "dependency.Start()", "dependency:=struct{Start,Stop func()}{Start:func(){},Stop:func(){}};dependency.Start()", 1),
		"shadowed-input":   strings.Replace(source, "host()\n os.Exit", "{var x *dependency.Data;_ = x};host()\n os.Exit", 1),
	} {
		t.Run(name, func(t *testing.T) {
			s.cache[sourcePath] = []byte(changed)
			defer func() { s.cache[sourcePath] = []byte(source) }()
			if _, e := Render(s, plan, dir); e == nil {
				t.Fatal("unsafe region accepted")
			}
		})
	}
	t.Run("value input", func(t *testing.T) {
		s.cache[depPath] = []byte(strings.ReplaceAll(dep, "Make()*Data", "Make()Data"))
		defer func() { s.cache[depPath] = []byte(dep) }()
		if _, e := Render(s, plan, dir); e == nil {
			t.Fatal("value input accepted")
		}
	})
	for _, inputs := range [][]string{{"x"}, {"x", "y", "absent"}, {"x", "x"}} {
		p := plan
		p.Units = append([]Unit(nil), plan.Units...)
		u := unit
		u.FunctionRegions = map[string]FunctionRegionRule{"Main": {First: unit.FunctionRegions["Main"].First, Last: unit.FunctionRegions["Main"].Last, Inputs: inputs}}
		p.Units[1] = u
		if _, e := Render(s, p, dir); e == nil {
			t.Fatal("invalid input declaration accepted", inputs)
		}
	}
}

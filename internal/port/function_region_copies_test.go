package port

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestNativeFunctionRegionCopies(t *testing.T) {
	const file = "src/cmd/compile/internal/p/main.go"
	const depFile = "src/cmd/internal/dependency/data.go"
	const source = `package p
import("os";"cmd/internal/dependency")
var total int
func Main(host func(),x *dependency.Data){
 host();os.Exit(2)
 for _,n:=range []int{1,2}{if dependency.Match(n,"start"){x.N+=n;total++}}
 // native front middle marker
 dependency.Middle()
 for _,n:=range []int{3}{if dependency.Match(n,"finish"){x.N+=10*n}}
 if dependency.Match(x.N,"tail"){x.N++}
 dependency.Stop()
 x.N*=2;total++
 dependency.End()
 host()
}
`
	const dep = `package dependency
type Data struct{N int}
func Match(n int,phase string)bool{return n>0}
func Middle(){}
func Stop(){}
func End(){}
`
	repo, pin := fixtureRepo(t, map[string]string{"LICENSE": "fixture", "PATENTS": "fixture", file: source, depFile: dep})
	s, e := OpenSource(context.Background(), repo, pin)
	if e != nil {
		t.Fatal(e)
	}
	first := SourceCallAnchor{Package: "cmd/internal/dependency", Member: "Match", Enclosing: "range", StringArgument: &SourceStringArgument{Index: 1, Value: "start"}}
	stop := SourceCallAnchor{Package: "cmd/internal/dependency", Member: "Stop"}
	u := Unit{Source: file, Functions: []string{"Main", "Frontend", "Conditional"}, Context: &ContextRule{Type: "Context", Constructor: "newContext", Selective: true, Adapter: "adapter_context.go", Fields: map[string]string{"total": "total"}}, ContextMethods: map[string]string{"Main": "tail", "Frontend": "front"}, FunctionRegions: map[string]FunctionRegionRule{
		"Conditional": {SourceFunction: "Main", First: SourceCallAnchor{Package: "cmd/internal/dependency", Member: "Match", Enclosing: "if", StringArgument: &SourceStringArgument{Index: 1, Value: "tail"}}, Last: stop, EndBefore: true, Inputs: []string{"x"}},
		"Main":        {First: stop, Last: SourceCallAnchor{Package: "cmd/internal/dependency", Member: "End"}, Inputs: []string{"x"}},
		"Frontend":    {SourceFunction: "Main", First: first, Last: stop, Inputs: []string{"x"}, EndBefore: true},
	}}
	plan := Plan{InspectRoots: []string{"cmd/compile/internal/p"}, Units: []Unit{{Source: depFile, WholeFile: true}, u}}
	dir := t.TempDir()
	writeTestFile(t, dir, "internal/p/adapter_context.go", `package p;type Context struct{total int}`)
	r, e := Render(s, plan, dir)
	if e != nil {
		t.Fatal(e)
	}
	if e = Install(dir, r); e != nil {
		t.Fatal(e)
	}
	data := string(r.Files["internal/p/main.go"])
	if strings.Contains(data, "host()") || strings.Contains(data, "os.Exit") || strings.Count(data, "dependency.Stop()") != 1 || strings.Count(data, "dependency.End()") != 1 || !strings.Contains(data, "x.N += 10 * n") {
		t.Fatal("copied interval shape", data)
	}
	if pos := strings.Index(data, "// native front middle marker"); pos < strings.Index(data, "func (syncgoCtx *Context) front(") {
		t.Fatal("copied source comment escaped its function", data)
	}
	provenance := false
	for _, f := range r.Ledger.Files {
		if strings.Contains(strings.Join(f.Declarations, ";"), "function-region-origin:Frontend:Main") {
			provenance = true
		}
	}
	if !provenance {
		t.Fatal("missing copied-source provenance")
	}
	writeTestFile(t, dir, "go.mod", "module go-backend-ssa\n\ngo 1.27.1\n")
	writeTestFile(t, dir, "internal/p/check_test.go", `package p
import("testing";"sync";"go-backend-ssa/internal/cmd/dependency")
func TestStages(t *testing.T){var wg sync.WaitGroup;for i:=range 8{wg.Go(func(){c:=newContext();x:=&dependency.Data{N:i};c.front(x);if x.N!=i+34||c.total!=2{t.Fatal("front",x,c.total)};c.tail(x);if x.N!=2*(i+34)||c.total!=3{t.Fatal("tail",x,c.total)};Conditional(x);if x.N!=2*(i+34)+1{t.Fatal("conditional",x)}})};wg.Wait()}
`)
	cmd := exec.Command("go", "test", "-race", "./internal/p")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOWORK=off", "GOTOOLCHAIN=go1.27.1")
	if b, e := cmd.CombinedOutput(); e != nil {
		t.Fatalf("%v\n%s", e, b)
	}
	for name, change := range map[string]string{
		"name collision":  source + "\nfunc Frontend(){}",
		"missing source":  strings.Replace(source, "func Main(", "func Other(", 1),
		"ambiguous range": strings.Replace(source, "dependency.Middle()", `for _,n:=range []int{1}{if dependency.Match(n,"start"){x.N++}}`, 1),
		"ambiguous call":  strings.Replace(source, `dependency.Match(n,"start")`, `dependency.Match(n,"start")||dependency.Match(n,"start")`, 1),
		"missing literal": strings.Replace(source, `dependency.Match(n,"start")`, `dependency.Match(n,"bad")`, 1),
		"return":          strings.Replace(source, "dependency.Middle()", "dependency.Middle();if x.N>3{return}", 1),
		"closure":         strings.Replace(source, "dependency.Middle()", "dependency.Middle();func(){x.N++}()", 1),
		"escape":          strings.Replace(source, "dependency.Middle()", "tmp:=1;dependency.Middle()", 1),
		"recursive":       strings.Replace(source, "dependency.Middle()", "Main(nil,x)", 1),
	} {
		if name == "escape" {
			change = strings.Replace(change, "x.N*=2", "x.N*=tmp", 1)
		}
		t.Run(name, func(t *testing.T) {
			s.cache[file] = []byte(change)
			defer func() { s.cache[file] = []byte(source) }()
			if _, e := Render(s, plan, dir); e == nil {
				t.Fatal("invalid source accepted")
			}
		})
	}
	for name, rule := range map[string]FunctionRegionRule{
		"empty":             {SourceFunction: "Main", First: stop, Last: stop, EndBefore: true},
		"bad enclosing":     {SourceFunction: "Main", First: SourceCallAnchor{Package: "cmd/internal/dependency", Member: "Match", Enclosing: "for"}, Last: stop},
		"negative argument": {SourceFunction: "Main", First: SourceCallAnchor{Package: "cmd/internal/dependency", Member: "Match", Enclosing: "range", StringArgument: &SourceStringArgument{Index: -1, Value: "start"}}, Last: stop},
	} {
		t.Run(name, func(t *testing.T) {
			bad := u
			bad.Functions = []string{"Main", "Frontend"}
			bad.FunctionRegions = map[string]FunctionRegionRule{"Main": u.FunctionRegions["Main"], "Frontend": rule}
			changed := plan
			changed.Units = []Unit{plan.Units[0], bad}
			if _, e := Render(s, changed, dir); e == nil {
				t.Fatal("bad region rule accepted")
			}
		})
	}
}

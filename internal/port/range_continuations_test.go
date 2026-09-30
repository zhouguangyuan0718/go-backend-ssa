package port

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestNativeRangeContinuation(t *testing.T) {
	const sourcePath = "src/cmd/compile/internal/p/main.go"
	const source = `package p
 type Data struct{N, Starts, Cleanups int; Values []int; Panic, Recovered any; Change bool}
 type pass struct{n int; skip bool}
 var passes = [4]pass{{1,false},{2,true},{3,false},{4,false}}
 func Main(f *Data) {
  serial := 7
  phase := "init"
  defer func(){if phase!="" {f.Recovered=recover()}}()
  f.Starts++
  if f.Change {passes[0].n=9}
  const scale = 3
  for _, p := range passes {
   if p.skip {continue}
   phase="pass"
   serial=serial*scale+p.n
   f.Values=append(f.Values,serial)
   if f.Panic!=nil {panic(f.Panic)}
   f.N+=p.n
  }
  f.Cleanups++
  phase=""
 }
 `
	const adapter = `package p
 type Context struct{passes [4]pass; hold bool; saved func()}
 func(c *Context)boundary(f *Data, run func(func([4]pass)[]pass,bool)) {
  if !c.hold {run(func(p [4]pass)[]pass{return p[:]},true);return}
  var snapshot [4]pass
  run(func(p [4]pass)[]pass{snapshot=p;return snapshot[:2]},true)
  c.saved=func(){run(func(_ [4]pass)[]pass{return snapshot[2:]},false)}
 }
 `
	plan := Plan{InspectRoots: []string{"cmd/compile/internal/p"}, Units: []Unit{{Source: sourcePath, WholeFile: true, Context: &ContextRule{Type: "Context", Constructor: "newContext", Selective: true, Adapter: "adapter_context.go", Init: true, Fields: map[string]string{"passes": "passes"}}, RangeContinuations: map[string]RangeContinuationRule{"Main": {Range: "passes", Method: "boundary", Argument: "f"}}}}}
	render := func(t *testing.T, code string) (*Rendered, string, error) {
		t.Helper()
		repo, pin := fixtureRepo(t, map[string]string{"LICENSE": "fixture", "PATENTS": "fixture", sourcePath: code})
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
	if err := Install(root, result); err != nil {
		t.Fatal(err)
	}
	seen := false
	for _, file := range result.Ledger.Files {
		if file.Output == "internal/p/main.go" {
			seen = strings.Contains(strings.Join(file.Declarations, ";"), "range-continuation:Main:passes:boundary:f")
		}
	}
	if !seen {
		t.Fatal("missing provenance")
	}
	writeTestFile(t, root, "go.mod", "module go-backend-ssa\n\ngo 1.27.1\n")
	writeTestFile(t, root, "internal/p/check_test.go", `package p
 import("testing";"slices";"fmt")
 func TestRunner(t *testing.T){for i:=range 8{t.Run(fmt.Sprint(i),func(t *testing.T){t.Parallel()
  c:=newContext();f:=new(Data);c.Main(f)
  if f.N!=8||f.Starts!=1||f.Cleanups!=1||!slices.Equal(f.Values,[]int{22,69,211}){t.Fatal(f)}
  c=newContext();c.hold=true;f=new(Data);c.Main(f)
  if f.N!=1||f.Starts!=1||f.Cleanups!=1||!slices.Equal(f.Values,[]int{22}){t.Fatal(f)}
  c.passes[2].n=999
  c.saved()
  if f.N!=8||f.Starts!=1||f.Cleanups!=2||!slices.Equal(f.Values,[]int{22,69,211}){t.Fatal(f)}
  c=newContext();c.hold=true;f=new(Data);c.Main(f);sentinel:=new(int);f.Panic=sentinel;c.saved()
  if f.Recovered!=sentinel||f.Cleanups!=1{t.Fatal("resumed native panic guard",f)}
  c=newContext();f=&Data{Change:true};c.Main(f);if f.N!=16||!slices.Equal(f.Values,[]int{30,93,283}){t.Fatal("snapshot evaluated before native prelude",f)}
 })}}
 `)
	cmd := exec.Command("go", "test", "-race", "./...")
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "GOTOOLCHAIN=go1.27.1", "GOWORK=off")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	for name, code := range map[string]string{
		"missing-loop":    strings.Replace(source, "range passes", "range [4]pass{}", 1),
		"slice-table":     strings.Replace(source, "var passes = [4]pass", "var passes = []pass", 1),
		"duplicate-loop":  strings.Replace(source, "f.Cleanups++", "for _, p := range passes {_=p};f.Cleanups++", 1),
		"return":          strings.Replace(source, "f.Cleanups++", "if f.N==0{return};f.Cleanups++", 1),
		"goto":            strings.Replace(source, "f.Cleanups++", "goto done;done:f.Cleanups++", 1),
		"local":           strings.Replace(source, "const scale = 3", "scale := 3", 1),
		"extra-defer":     strings.Replace(source, "f.Cleanups++", "defer func(){}();f.Cleanups++", 1),
		"shadow-arg":      strings.Replace(source, "f.Starts++", "if true{f:=1;_=f};f.Starts++", 1),
		"constant-shadow": strings.Replace(strings.Replace(source, "var passes", "const scale=9\n var passes", 1), "f.Starts++", "f.Starts+=scale", 1),
		"reserved-name":   strings.Replace(source, "serial := 7", "syncgoStageStart:=true;_=syncgoStageStart;serial := 7", 1),
	} {
		t.Run(name, func(t *testing.T) {
			if _, _, err := render(t, code); err == nil {
				t.Fatal("unsafe range continuation accepted")
			}
		})
	}
}

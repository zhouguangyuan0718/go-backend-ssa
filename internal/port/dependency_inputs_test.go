package port

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestInitializedDependencyInputs(t *testing.T) {
	const model = `package model
 type Type struct{ID int}
 var Types=[]*Type{{ID:1}}
 func Canonical()*Type{return Types[0]}
 `
	const source = `package p
 import "cmd/compile/internal/model"
 var cached=model.Types[0]
 func Same()bool{return cached==model.Canonical()}
 func Callback()func()*model.Type{return model.Canonical}
 // This comment describes //go:linkname, but is not a compiler directive.
 `
	repo, pin := fixtureRepo(t, map[string]string{"LICENSE": "fixture", "PATENTS": "fixture", "src/cmd/compile/internal/model/model.go": model, "src/cmd/compile/internal/p/p.go": source})
	s, err := OpenSource(context.Background(), repo, pin)
	if err != nil {
		t.Fatal(err)
	}
	plan := Plan{InspectRoots: []string{"cmd/compile/internal/p"}, Units: []Unit{
		{Source: "src/cmd/compile/internal/model/model.go", WholeFile: true, Context: &ContextRule{Type: "Context", Constructor: "NewContext", Adapter: "adapter_context.go", Fields: map[string]string{"Types": "Types"}}},
		{Source: "src/cmd/compile/internal/p/p.go", WholeFile: true, Context: &ContextRule{Type: "Context", Constructor: "initialize", Adapter: "adapter_context.go", Initialize: true, Fields: map[string]string{"cached": "cached"}}, State: []StateRule{{Source: "src/cmd/compile/internal/model/model.go", Variable: "Types", Field: "Model.Types"}}, Calls: []CallRule{{Source: "src/cmd/compile/internal/model/model.go", Function: "Canonical", Method: "Model.Canonical"}}},
	}}
	dir := t.TempDir()
	writeTestFile(t, dir, "internal/model/adapter_context.go", `package model;type Context struct{Types []*Type}`)
	const adapter = `package p
 import "go-backend-ssa/internal/model"
 type Context struct{Model *model.Context;cached *model.Type}
 func NewContext(m *model.Context)*Context{c:=&Context{Model:m};c.initialize();return c}
 `
	writeTestFile(t, dir, "internal/p/adapter_context.go", adapter)
	r, err := Render(s, plan, dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := Install(dir, r); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, dir, "go.mod", "module go-backend-ssa\n\ngo 1.27.1\n")
	writeTestFile(t, dir, "internal/p/context_test.go", `package p
 import("testing";"sync";"go-backend-ssa/internal/model")
 func TestDependencies(t *testing.T){a,b:=model.NewContext(),model.NewContext();x,y:=NewContext(a),NewContext(b);callback:=x.Callback();if !x.Same()||!y.Same()||x.cached==y.cached||callback()!=a.Canonical(){t.Fatal("split dependency identity")};var wg sync.WaitGroup;for range 16{wg.Go(func(){m:=model.NewContext();c:=NewContext(m);if !c.Same()||c.Callback()()!=m.Canonical(){t.Error("concurrent dependency")}})};wg.Wait()}
 `)
	cmd := exec.Command("go", "test", "-race", "./...")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOWORK=off", "GOTOOLCHAIN=go1.27.1")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	// An actual directive must remain rejected despite prose comments being OK.
	s.cache[plan.Units[1].Source] = []byte(source + "\n//go:linkname Same foreign.Same\n")
	if _, err := Render(s, plan, dir); err == nil || !strings.Contains(err.Error(), "linkname") {
		t.Fatalf("directive accepted: %v", err)
	}
}

func TestLocalDriverBindings(t *testing.T) {
	const source = `package p
 type Item struct{N int}
 func Build(i *Item)int{Input(i);return i.N}
 `
	const driver = `package p
 func Input(i *Item){panic("driver body must not be copied")}
 `
	repo, pin := fixtureRepo(t, map[string]string{"LICENSE": "fixture", "PATENTS": "fixture", "src/cmd/compile/internal/p/core.go": source, "src/cmd/compile/internal/p/driver.go": driver})
	s, err := OpenSource(context.Background(), repo, pin)
	if err != nil {
		t.Fatal(err)
	}
	plan := Plan{InspectRoots: []string{"cmd/compile/internal/p"}, Units: []Unit{{Source: "src/cmd/compile/internal/p/core.go", WholeFile: true, Context: &ContextRule{Type: "Context", Constructor: "NewContext", Adapter: "adapter_context.go"}, Calls: []CallRule{{Source: "src/cmd/compile/internal/p/driver.go", Function: "Input", Method: "input"}}}}}
	dir := t.TempDir()
	writeTestFile(t, dir, "internal/p/adapter_context.go", `package p;type Context struct{N int};func(c *Context)input(i *Item){i.N=c.N}`)
	r, err := Render(s, plan, dir)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(r.Files[plan.Units[0].output()]), "func Input") {
		t.Fatal("prototype emitted into library")
	}
	if err := Install(dir, r); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, dir, "go.mod", "module go-backend-ssa\n\ngo 1.27.1\n")
	writeTestFile(t, dir, "internal/p/context_test.go", `package p;import "testing";func TestBind(t *testing.T){c:=NewContext();c.N=7;if c.Build(&Item{})!=7{t.Fatal("local binding")}}`)
	cmd := exec.Command("go", "test", "./...")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOWORK=off", "GOTOOLCHAIN=go1.27.1")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	// The same binding also removes a selected original helper body.
	plan.Units = append(plan.Units, Unit{Source: "src/cmd/compile/internal/p/driver.go", WholeFile: true})
	selected, err := Render(s, plan, dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, data := range selected.Files {
		if strings.Contains(string(data), "func Input") || strings.Contains(string(data), "driver body must not be copied") {
			t.Fatal("bound selected helper emitted")
		}
	}
	plan.Units[1].Context = plan.Units[0].Context
	if _, err := Render(s, plan, dir); err != nil {
		t.Fatalf("binding overrides generated method: %v", err)
	}
	plan.Units[0].Calls[0].Source = plan.Units[0].Source
	if _, err := Render(s, plan, dir); err == nil || !strings.Contains(err.Error(), "local binding source mismatch") {
		t.Fatalf("wrong source accepted: %v", err)
	}
	plan.Units[0].Calls[0].Source = "src/cmd/compile/internal/p/driver.go"
	plan.Units = plan.Units[:1]
	s.cache["src/cmd/compile/internal/p/driver.go"] = []byte(strings.Replace(driver, "Input(i *Item)", "Input(i *Item) bool", 1))
	if _, err := Render(s, plan, dir); err == nil || !strings.Contains(err.Error(), "signature mismatch") {
		t.Fatalf("local signature drift: %v", err)
	}
}

func TestAdditionalDriverSignaturesPreserveTypes(t *testing.T) {
	const model = `package model;type Item struct{N int};func Known(i *Item)int{return i.N};func Host(i *Item)int{panic("host")}`
	const source = `package p;import "cmd/compile/internal/model";func Run(i *model.Item)int{return model.Known(i)+model.Host(i)}`
	repo, pin := fixtureRepo(t, map[string]string{"LICENSE": "fixture", "PATENTS": "fixture", "src/cmd/compile/internal/model/model.go": model, "src/cmd/compile/internal/p/p.go": source})
	s, err := OpenSource(context.Background(), repo, pin)
	if err != nil {
		t.Fatal(err)
	}
	plan := Plan{InspectRoots: []string{"cmd/compile/internal/p"}, Units: []Unit{
		{Source: "src/cmd/compile/internal/model/model.go", Declarations: []string{"Item"}, Functions: []string{"Known"}},
		{Source: "src/cmd/compile/internal/p/p.go", WholeFile: true, Context: &ContextRule{Type: "Context", Constructor: "NewContext", Adapter: "adapter_context.go"}, Calls: []CallRule{
			{Source: "src/cmd/compile/internal/model/model.go", Function: "Known", Method: "known"},
			{Source: "src/cmd/compile/internal/model/model.go", Function: "Host", Method: "host"},
		}},
	}}
	dir := t.TempDir()
	writeTestFile(t, dir, "internal/p/adapter_context.go", `package p;import "go-backend-ssa/internal/model";type Context struct{};func(c *Context)known(i *model.Item)int{return model.Known(i)};func(c *Context)host(i *model.Item)int{return 2*i.N}`)
	r, err := Render(s, plan, dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := Install(dir, r); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, dir, "go.mod", "module go-backend-ssa\n\ngo 1.27.1\n")
	writeTestFile(t, dir, "internal/p/context_test.go", `package p;import("testing";"go-backend-ssa/internal/model");func TestBind(t *testing.T){if NewContext().Run(&model.Item{N:3})!=9{t.Fatal("binding")}}`)
	cmd := exec.Command("go", "test", "./...")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOWORK=off", "GOTOOLCHAIN=go1.27.1")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	s.cache[plan.Units[0].Source] = []byte(strings.Replace(model, "Host(i *Item)int", "Host(i *Item)bool", 1))
	if _, err := Render(s, plan, dir); err == nil {
		t.Fatal("signature drift accepted")
	}
}

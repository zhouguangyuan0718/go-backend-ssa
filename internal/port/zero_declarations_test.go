package port

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestLocalZeroValueBinding(t *testing.T) {
	const modelPath = "src/cmd/compile/internal/model/model.go"
	const sourcePath = "src/cmd/compile/internal/p/p.go"
	const source = `package p
 import renamed "cmd/compile/internal/model"
 type state struct{}
 func(s *state)zero()renamed.Value{var v renamed.Value;return v}
 func Method()renamed.Value{return new(state).zero()}
 func Zero()renamed.Value{var v renamed.Value;return v}
 func Caller()renamed.Value{return Zero()}
 func Many()[]renamed.Value{var a,b renamed.Value;return []renamed.Value{a,b}}
 func Closure()func()renamed.Value{return func()renamed.Value{var v renamed.Value;return v}}
 func Loop()[]renamed.Value{var out []renamed.Value;for range 3{var v renamed.Value;out=append(out,v)};return out}
 func Alias()renamed.Value{type Local=renamed.Value;var v Local;return v}
 func Shadow()int{type Value struct{N int};var v Value;return v.N}
 func Pointer()*renamed.Value{var v *renamed.Value;return v}
 `
	repo, pin := fixtureRepo(t, map[string]string{"LICENSE": "fixture", "PATENTS": "fixture", modelPath: `package model;type Value struct{N int;Values map[int]int}`, sourcePath: source})
	s, err := OpenSource(context.Background(), repo, pin)
	if err != nil {
		t.Fatal(err)
	}
	plan := Plan{InspectRoots: []string{"cmd/compile/internal/p"}, Units: []Unit{
		{Source: modelPath, WholeFile: true},
		{Source: sourcePath, WholeFile: true, Context: &ContextRule{Type: "Context", Constructor: "newContext", Selective: true, Adapter: "adapter_context.go"}, ContextParams: []string{"state.zero"}, Literals: []LiteralRule{{Source: modelPath, Type: "Value", Method: "bind"}}},
	}}
	dir := t.TempDir()
	writeTestFile(t, dir, "internal/p/adapter_context.go", `package p;import "go-backend-ssa/internal/model"
 type Context struct{calls int;id int}
 func(c *Context)bind(v model.Value)model.Value{if v.N!=0||v.Values!=nil{panic("nonzero input")};c.calls++;v.N=c.id;return v}
 `)
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
 func TestZero(t *testing.T){old:=newContext();old.id=99;f:=old.Closure();var wg sync.WaitGroup;for i:=range 8{wg.Go(func(){c:=newContext();c.id=i+1;vs:=c.Many();vs=append(vs,c.Caller(),c.Alias(),c.Method());vs=append(vs,c.Loop()...);for _,v:=range vs{if v.N!=i+1||v.Values!=nil{t.Error(v)}};if c.calls!=8||Shadow()!=0||Pointer()!=nil{t.Error(c)}})};wg.Wait();v:=f();if old.calls!=1||v.N!=99||v.Values!=nil{t.Fatal(old,v)}}
 `)
	cmd := exec.Command("go", "test", "-race", "./...")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOWORK=off", "GOTOOLCHAIN=go1.27.1")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	// Package state needs an explicit per-instance initializer. Do not quietly
	// attach a compilation to a process-wide zero value.
	s.cache[sourcePath] = []byte(source + "\nvar global renamed.Value\n")
	if _, err := Render(s, plan, dir); err == nil || !strings.Contains(err.Error(), "local") {
		t.Fatalf("package zero accepted: %v", err)
	}
}

package port

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestLiteralBindingsPreserveValuesAndIdentity(t *testing.T) {
	const modelPath = "src/cmd/compile/internal/model/model.go"
	const sourcePath = "src/cmd/compile/internal/p/p.go"
	model := `package model;type Value struct{N int;Next *Value}`
	source := `package p
 import renamed "cmd/compile/internal/model"
 func Build(next func()int)renamed.Value{return renamed.Value{N:next()}}
 func Caller(next func()int)renamed.Value{return Build(next)}
 func Nested()renamed.Value{return renamed.Value{N:Build(func()int{return 2}).N}}
 func NestedLiterals()renamed.Value{return renamed.Value{N:[]renamed.Value{renamed.Value{N:1}}[0].N}}
 func Shadow()int{type Value struct{N int};return (Value{N:7}).N}
 `
	repo, pin := fixtureRepo(t, map[string]string{"LICENSE": "fixture", "PATENTS": "fixture", modelPath: model, sourcePath: source})
	s, err := OpenSource(context.Background(), repo, pin)
	if err != nil {
		t.Fatal(err)
	}
	plan := Plan{InspectRoots: []string{"cmd/compile/internal/p"}, Units: []Unit{
		{Source: modelPath, WholeFile: true},
		{Source: sourcePath, WholeFile: true, Context: &ContextRule{Type: "Context", Constructor: "newContext", Selective: true, Adapter: "adapter_context.go"}, Literals: []LiteralRule{{Source: modelPath, Type: "Value", Method: "bind"}}},
	}}
	dir := t.TempDir()
	adapter := `package p;import "go-backend-ssa/internal/model"
 type Context struct{calls int}
 func(c *Context)bind(v model.Value)model.Value{c.calls++;v.N+=10;return v}
 `
	writeTestFile(t, dir, "internal/p/adapter_context.go", adapter)
	r, err := Render(s, plan, dir)
	if err != nil {
		t.Fatal(err)
	}
	if r.Ledger.Files[1].SignatureSources[modelPath] != hash([]byte(model)) {
		t.Fatal("literal source not recorded")
	}
	if err := Install(dir, r); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, dir, "go.mod", "module go-backend-ssa\n\ngo 1.27.1\n")
	writeTestFile(t, dir, "internal/p/adapter_test.go", `package p
 import "testing"
 func TestValues(t *testing.T){a,b:=newContext(),newContext();n:=0;v:=a.Caller(func()int{n++;return n});if n!=1||v.N!=11||a.calls!=1||b.calls!=0||Shadow()!=7{t.Fatal(n,v,a,b)};if a.Nested().N!=22||a.calls!=3{t.Fatal(a)};if a.NestedLiterals().N!=21||a.calls!=5{t.Fatal(a)}}
 `)
	cmd := exec.Command("go", "test", "-race", "./...")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOWORK=off", "GOTOOLCHAIN=go1.27.1")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	// This broader argument still compiles; the contract must reject it.
	writeTestFile(t, dir, "internal/p/adapter_context.go", strings.Replace(adapter, "bind(v model.Value)model.Value{", "bind(input any)model.Value{v:=input.(model.Value);", 1))
	if _, err := Render(s, plan, dir); err == nil || !strings.Contains(err.Error(), "literal adapter type mismatch") {
		t.Fatalf("adapter drift accepted: %v", err)
	}
	writeTestFile(t, dir, "internal/p/adapter_context.go", adapter)
	s.cache[sourcePath] = []byte(strings.Replace(source, "return renamed.Value{N:next()}", "return *(&renamed.Value{N:next()})", 1))
	if _, err := Render(s, plan, dir); err == nil || !strings.Contains(err.Error(), "not an address") {
		t.Fatalf("pointer literal accepted: %v", err)
	}
	s.cache[sourcePath] = []byte(source + `
func Elided()[]*renamed.Value{return []*renamed.Value{{N:1}}}`)
	if _, err := Render(s, plan, dir); err == nil || !strings.Contains(err.Error(), "explicit non-generic struct") {
		t.Fatalf("implicit pointer literal accepted: %v", err)
	}
	s.cache[sourcePath] = []byte(`package p;func Pure()int{return 1}`)
	if _, err := Render(s, plan, dir); err == nil || !strings.Contains(err.Error(), "not used") {
		t.Fatalf("unused rule accepted: %v", err)
	}
}

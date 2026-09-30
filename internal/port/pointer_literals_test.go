package port

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestPointerLiteralBinding(t *testing.T) {
	const modelPath = "src/cmd/compile/internal/model/model.go"
	const sourcePath = "src/cmd/compile/internal/p/p.go"
	const source = `package p
 import m "cmd/compile/internal/model"
 func Build(next func()int)*m.Value{return &(m.Value{N:next(),Next:&m.Value{N:next()}})}
 func Closure()func()*m.Value{return func()*m.Value{return &m.Value{}}}
 func Shadow()int{type Value struct{N int};return (&Value{N:7}).N}
 `
	repo, pin := fixtureRepo(t, map[string]string{"LICENSE": "fixture", "PATENTS": "fixture", modelPath: `package model;type Value struct{N int;Next *Value;Owner int}`, sourcePath: source})
	s, err := OpenSource(context.Background(), repo, pin)
	if err != nil {
		t.Fatal(err)
	}
	plan := Plan{InspectRoots: []string{"cmd/compile/internal/p"}, Units: []Unit{
		{Source: modelPath, WholeFile: true},
		{Source: sourcePath, WholeFile: true, Context: &ContextRule{Type: "Context", Constructor: "newContext", Selective: true, Adapter: "adapter_context.go"}, Literals: []LiteralRule{{Source: modelPath, Type: "Value", Method: "bind", Pointer: true}}},
	}}
	dir := t.TempDir()
	adapter := `package p;import "go-backend-ssa/internal/model"
 type Context struct{id int;seen []*model.Value}
 func(c *Context)bind(v *model.Value)*model.Value{c.seen=append(c.seen,v);v.Owner=c.id;return v}
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
	writeTestFile(t, dir, "internal/p/adapter_test.go", `package p
 import("testing";"sync")
 func TestPointer(t *testing.T){old:=newContext();old.id=99;f:=old.Closure();var wg sync.WaitGroup;for i:=range 8{wg.Go(func(){c:=newContext();c.id=i+1;calls:=0;v:=c.Build(func()int{calls++;return calls});if calls!=2||len(c.seen)!=2||v.N!=1||v.Next.N!=2||v.Owner!=i+1||v.Next.Owner!=i+1||c.seen[0]!=v.Next||c.seen[1]!=v||Shadow()!=7{t.Error(c,v)}})};wg.Wait();v:=f();if v.Owner!=99||old.seen[0]!=v{t.Fatal("closure ownership")}}
 `)
	cmd := exec.Command("go", "test", "-race", "./...")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOWORK=off", "GOTOOLCHAIN=go1.27.1")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	for _, extra := range []string{`func Value()m.Value{return m.Value{}}`, `func Elided()[]*m.Value{return []*m.Value{{}}}`, `func Zero()m.Value{var v m.Value;return v}`} {
		s.cache[sourcePath] = []byte(source + "\n" + extra)
		if _, err := Render(s, plan, dir); err == nil {
			t.Fatalf("unsupported construction accepted: %s", extra)
		}
	}
	s.cache[sourcePath] = []byte(source)
	writeTestFile(t, dir, "internal/p/adapter_context.go", strings.Replace(adapter, "bind(v *model.Value)*model.Value{", "bind(input any)*model.Value{v:=input.(*model.Value);", 1))
	if _, err := Render(s, plan, dir); err == nil || !strings.Contains(err.Error(), "literal adapter type mismatch") {
		t.Fatalf("pointer adapter drift: %v", err)
	}
}

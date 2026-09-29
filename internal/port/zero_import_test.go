package port

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestImportedStructZeroValues(t *testing.T) {
	const source = `package p
 import "go/token"
 var NoPos token.Position
 var nopos token.Position
 func Pointer()*token.Position{return &nopos}
 type Context struct{}
 func NewContext()*Context{return &Context{}}
 `
	repo, pin := fixtureRepo(t, map[string]string{"LICENSE": "fixture", "PATENTS": "fixture", "src/cmd/compile/internal/p/a.go": source, "src/cmd/compile/internal/p/b.go": `package p;func Known()bool{return nopos.IsValid()}`, "src/cmd/compile/internal/q/q.go": `package q;import "cmd/compile/internal/p";func Value()any{return p.NoPos};func Copy()any{return &p.NoPos}`})
	s, err := OpenSource(context.Background(), repo, pin)
	if err != nil {
		t.Fatal(err)
	}
	plan := Plan{InspectRoots: []string{"cmd/compile/internal/q"}, Units: []Unit{
		{Source: "src/cmd/compile/internal/p/a.go", WholeFile: true, ZeroValues: []string{"NoPos", "nopos"}, Context: &ContextRule{Existing: true, Selective: true, Type: "Context", Constructor: "NewContext"}},
		{Source: "src/cmd/compile/internal/p/b.go", WholeFile: true},
		{Source: "src/cmd/compile/internal/q/q.go", WholeFile: true},
	}}
	dir := t.TempDir()
	r, err := Render(s, plan, dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := Install(dir, r); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, dir, "go.mod", "module go-backend-ssa\n\ngo 1.27.1\n")
	writeTestFile(t, dir, "internal/q/adapter_test.go", `package q
 import("go/token";"testing";"go-backend-ssa/internal/p")
 func TestZeros(t *testing.T){a,b:=p.Pointer(),p.Pointer();a.Line=4;if a==b||b.Line!=0||p.Known(){t.Fatal("shared zero")};if v,ok:=Value().(token.Position);!ok||v!=(token.Position{}){t.Fatal("type identity")};if _,ok:=Copy().(*token.Position);!ok{t.Fatal("pointer identity")}}
 `)
	cmd := exec.Command("go", "test", "-race", "./...")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOWORK=off", "GOTOOLCHAIN=go1.27.1")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	s.cache[plan.Units[0].Source] = []byte(source + "\nfunc Shadow() {zeroValue_nopos := 1; _ = zeroValue_nopos}\n")
	if _, err := Render(s, plan, t.TempDir()); err == nil || !strings.Contains(err.Error(), "alias collision") {
		t.Fatalf("got %v", err)
	}
}

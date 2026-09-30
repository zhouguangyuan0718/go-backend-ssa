package port

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestBoundSliceAllocation(t *testing.T) {
	const path = "src/cmd/compile/internal/p/p.go"
	const source = `package p;var calls int;type Cell struct{N int};func(x *Cell)Step()int{calls++;x.N++;return x.N};func size(n int)int{calls++;return n};func Allocate(n,capacity int)[]Cell{return make([]Cell,size(n),size(capacity))};func Equal(n int)[]Cell{return make([]Cell,size(n))}`
	const adapter = `package p;type Context struct{Calls,Bindings int;First *Cell};func(c *Context)bind(v []Cell)[]Cell{c.Bindings++;full:=v[:cap(v)];if len(full)>0{c.First=&full[0]};for i:=range full{full[i].compilation=c};return v}`
	repo, pin := fixtureRepo(t, map[string]string{"LICENSE": "fixture", "PATENTS": "fixture", path: source})
	s, err := OpenSource(context.Background(), repo, pin)
	if err != nil {
		t.Fatal(err)
	}
	rule := LiteralRule{Source: path, Type: "Cell", Method: "bind", MakeSlice: true}
	plan := Plan{InspectRoots: []string{"cmd/compile/internal/p"}, Units: []Unit{{Source: path, WholeFile: true, Context: &ContextRule{Type: "Context", Constructor: "initialize", Initialize: true, Selective: true, Adapter: "adapter_context.go", Fields: map[string]string{"calls": "Calls"}}, Owner: &OwnerRule{Types: []string{"Cell"}, Context: "Context", Field: "compilation"}, Literals: []LiteralRule{rule}}}}
	dir := t.TempDir()
	writeTestFile(t, dir, "internal/p/adapter_context.go", adapter)
	r, err := Render(s, plan, dir)
	if err != nil {
		t.Fatal(err)
	}
	if err = Install(dir, r); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, dir, "go.mod", "module go-backend-ssa\n\ngo 1.27.1\n")
	writeTestFile(t, dir, "internal/p/check_test.go", `package p;import("sync";"testing");func TestAllocation(t *testing.T){var wg sync.WaitGroup;for range 8{wg.Go(func(){c:=new(Context);v:=c.Allocate(2,5);if len(v)!=2||cap(v)!=5||c.Calls!=2||c.Bindings!=1||&v[0]!=c.First{t.Error("allocation changed")};full:=v[:cap(v)];for i:=range full{if full[i].Step()!=1{t.Error("element changed")}};if c.Calls!=7{t.Error("element owner missing")};empty:=c.Allocate(0,0);if empty==nil||len(empty)!=0||cap(empty)!=0{t.Error("zero allocation changed")};if len(c.Equal(3))!=3{t.Error("two-argument make changed")};before:=c.Bindings;func(){defer func(){if recover()==nil{t.Error("negative make accepted")}}();c.Allocate(-1,2)}();if c.Bindings!=before{t.Error("binder ran before allocation failure")}})};wg.Wait()}`)
	cmd := exec.Command("go", "test", "-race", "./...")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOWORK=off", "GOTOOLCHAIN=go1.27.1")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	for name, code := range map[string]string{
		"literal":          `package p;var calls int;type Cell struct{N int};func Allocate()[]Cell{return []Cell{{N:1}}}`,
		"pointer elements": `package p;var calls int;type Cell struct{N int};func Allocate()[]*Cell{return make([]*Cell,2)}`,
		"named slice":      `package p;var calls int;type Cell struct{N int};type Cells []Cell;func Allocate()Cells{return make(Cells,2)}`,
		"shadowed builtin": `package p;var calls int;type Cell struct{N int};func Allocate()[]Cell{make:=func(int,int)[]Cell{return nil};return make(2,3)}`,
	} {
		t.Run(name, func(t *testing.T) {
			s.cache[path] = []byte(code)
			defer func() { s.cache[path] = []byte(source) }()
			if _, err := Render(s, plan, dir); err == nil || !strings.Contains(err.Error(), "literal binding not used") {
				t.Fatalf("unreviewed allocation accepted: %v", err)
			}
		})
	}
	plan.Units[0].Literals = append(plan.Units[0].Literals, rule)
	if _, err := Render(s, plan, dir); err == nil || !strings.Contains(err.Error(), "duplicate slice allocation") {
		t.Fatalf("duplicate accepted: %v", err)
	}
	plan.Units[0].Literals = []LiteralRule{rule}
	plan.Units[0].Literals[0].NilPointer = true
	if err := plan.Validate(); err == nil {
		t.Fatal("incompatible rules accepted")
	}
	plan.Units[0].Literals = []LiteralRule{rule}
	// Keep the widened adapter type-correct so the signature audit is exercised.
	writeTestFile(t, dir, "internal/p/adapter_context.go", `package p;type Context struct{Calls int};func(c *Context)bind(input any)[]Cell{return input.([]Cell)}`)
	if _, err := Render(s, plan, dir); err == nil || !strings.Contains(err.Error(), "literal adapter type mismatch") {
		t.Fatalf("widened binder accepted: %v", err)
	}
}

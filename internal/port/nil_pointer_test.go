package port

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestBoundNilPointer(t *testing.T) {
	const source = `package p;import "cmd/compile/internal/q";func Resolve()*q.Pkg{return (*q.Pkg)(nil)};func Access()int{return ((*q.Pkg)((nil))).Read()}`
	const dependency = `package q;type Pkg struct{N int};func(p *Pkg)Read()int{if p==nil{return -1};p.N++;return p.N}`
	const adapter = `package p;import "go-backend-ssa/internal/q";type Context struct{Default q.Pkg;Calls int};func(c *Context)bind(p *q.Pkg)*q.Pkg{if p!=nil{panic("non-nil bound")};c.Calls++;return &c.Default}`
	const path = "src/cmd/compile/internal/p/p.go"
	repo, pin := fixtureRepo(t, map[string]string{"LICENSE": "fixture", "PATENTS": "fixture", path: source, "src/cmd/compile/internal/q/q.go": dependency})
	s, err := OpenSource(context.Background(), repo, pin)
	if err != nil {
		t.Fatal(err)
	}
	rule := LiteralRule{Source: "src/cmd/compile/internal/q/q.go", Type: "Pkg", Method: "bind", NilPointer: true}
	plan := Plan{InspectRoots: []string{"cmd/compile/internal/p", "cmd/compile/internal/q"}, Units: []Unit{{Source: path, WholeFile: true, Context: &ContextRule{Type: "Context", Constructor: "initialize", Initialize: true, Selective: true, Adapter: "adapter_context.go"}, Literals: []LiteralRule{rule}}, {Source: "src/cmd/compile/internal/q/q.go", WholeFile: true}}}
	plan.Units[0], plan.Units[1] = plan.Units[1], plan.Units[0]
	dir := t.TempDir()
	writeTestFile(t, dir, "internal/p/adapter_context.go", adapter)
	r, err := Render(s, plan, dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := Install(dir, r); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, dir, "go.mod", "module go-backend-ssa\n\ngo 1.27.1\n")
	writeTestFile(t, dir, "internal/p/check_test.go", `package p;import("sync";"testing");func TestContexts(t *testing.T){var wg sync.WaitGroup;for i:=range 8{wg.Go(func(){c:=new(Context);c.Default.N=i;if c.Resolve()!=&c.Default||c.Access()!=i+1||c.Calls!=2{t.Error("nil binding lost context or receiver effects")}})};wg.Wait()}`)
	cmd := exec.Command("go", "test", "-race", "./...")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOWORK=off", "GOTOOLCHAIN=go1.27.1")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	for name, code := range map[string]string{
		"non-nil conversion":     `package p;import "cmd/compile/internal/q";func Resolve(p *q.Pkg)*q.Pkg{return (*q.Pkg)(p)}`,
		"untyped nil":            `package p;import "cmd/compile/internal/q";func Resolve()*q.Pkg{return nil}`,
		"shadowed nil":           `package p;import "cmd/compile/internal/q";func Resolve()*q.Pkg{nil:=&q.Pkg{};return (*q.Pkg)(nil)}`,
		"call returning pointer": `package p;import "cmd/compile/internal/q";func makePkg(p *q.Pkg)*q.Pkg{return p};func Resolve()*q.Pkg{return makePkg(nil)}`,
	} {
		t.Run(name, func(t *testing.T) {
			s.cache[path] = []byte(code)
			defer func() { s.cache[path] = []byte(source) }()
			if _, err := Render(s, plan, dir); err == nil || !strings.Contains(err.Error(), "literal binding not used") {
				t.Fatalf("unreviewed expression: %v", err)
			}
		})
	}
	plan.Units[1].Literals = append(plan.Units[1].Literals, rule)
	if _, err := Render(s, plan, dir); err == nil || !strings.Contains(err.Error(), "duplicate nil pointer") {
		t.Fatalf("duplicate accepted: %v", err)
	}
	plan.Units[1].Literals = []LiteralRule{rule}
	plan.Units[1].Literals[0].Pointer = true
	if err := plan.Validate(); err == nil {
		t.Fatal("incompatible pointer rules accepted")
	}
	plan.Units[1].Literals = []LiteralRule{rule}
	writeTestFile(t, dir, "internal/p/adapter_context.go", strings.Replace(adapter, "bind(p *q.Pkg)", "bind(p any)", 1))
	if _, err := Render(s, plan, dir); err == nil || !strings.Contains(err.Error(), "literal adapter type mismatch") {
		t.Fatalf("widened binder signature accepted: %v", err)
	}
	writeTestFile(t, dir, "internal/p/adapter_context.go", adapter)
	plan.Units[1].Literals[0].Method = "missing"
	if _, err := Render(s, plan, dir); err == nil {
		t.Fatal("missing binder accepted")
	}
}

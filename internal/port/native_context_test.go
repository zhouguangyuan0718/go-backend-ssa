package port

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestNativeContext(t *testing.T) {
	const original = `package p
 import "sync"
 // Context keeps the original synchronization and caches.
 type Context struct {
  mu sync.Mutex // cache lock
  cache map[int]int
 }
 // NewContext allocates the original caches.
 func NewContext() *Context { return &Context{cache: make(map[int]int)} }
 func(c *Context) Remember(n int) int {c.mu.Lock();defer c.mu.Unlock();c.cache[n]=len(table);return c.cache[n]}
 func(c *Context) Spawn()*Context{return NewContext()}
 `
	const state = `package p
 import value "go/constant"
 type Basic struct{kind int}
 var table = [...]*Basic{{1},{2}}
 var answer value.Value = value.MakeInt64(42)
 var order []int
 func mark(n int)int{order=append(order,n);return n}
 var second=mark(first+1)
 var first=mark(1)
 func init(){if len(order)!=2||order[0]!=1||order[1]!=2{panic("initializer order")};table[0].kind=second}
 func(c *Context) Answer()value.Value {return answer}
 func(c *Context) Tables()[]*Basic{return table[:]}
 func(b *Basic) Next()*Basic {return &Basic{b.kind+first}}
 func Snapshot()int{return table[0].kind}
 `
	plan := Plan{InspectRoots: []string{"cmd/compile/internal/p"}, Units: []Unit{
		{Source: "src/cmd/compile/internal/p/context.go", WholeFile: true, Context: &ContextRule{Existing: true, Type: "Context", Constructor: "NewContext"}},
		{Source: "src/cmd/compile/internal/p/state.go", WholeFile: true, Owner: &OwnerRule{Types: []string{"Basic"}, Context: "Context", Field: "owner"}, Context: &ContextRule{Existing: true, Type: "Context", Constructor: "NewContext", Init: true, Fields: map[string]string{"table": "table", "answer": "answer", "order": "order", "first": "first", "second": "second"}}},
	}}
	render := func(t *testing.T, ctx string, plan Plan) (*Rendered, string, error) {
		t.Helper()
		repo, pin := fixtureRepo(t, map[string]string{"LICENSE": "fixture", "PATENTS": "fixture", plan.Units[0].Source: ctx, plan.Units[1].Source: state})
		s, err := OpenSource(context.Background(), repo, pin)
		if err != nil {
			t.Fatal(err)
		}
		dir := t.TempDir()
		r, err := Render(s, plan, dir)
		return r, dir, err
	}
	r, dir, err := render(t, original, plan)
	if err != nil {
		t.Fatal(err)
	}
	if err := Install(dir, r); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, dir, "go.mod", "module go-backend-ssa\n\ngo 1.27.1\n")
	writeTestFile(t, dir, "internal/p/adapter_test.go", `package p
 import("testing";"sync";"go/constant")
 func TestContexts(t *testing.T){a:=NewContext();old:=a.Tables()[0];if old.owner!=a||old.Next().kind!=3||old.Next().owner!=a||a.Remember(3)!=2||a.Snapshot()!=2||a.Answer().ExactString()!="42"{t.Fatal("source behavior")}; b:=a.Spawn();a.answer=constant.MakeInt64(7);a.table[0].kind=99;if b.answer.ExactString()!="42"||b.table[0].kind!=2||b.table[0]==old||old.kind!=99{t.Fatal("instance isolation or retained graph")};var wg sync.WaitGroup;for range 16{wg.Go(func(){c:=NewContext();if c.Remember(1)!=2||c.Tables()[0].owner!=c||c.Answer().ExactString()!="42"{t.Error("concurrent instance")}})};wg.Wait()}
 `)
	cmd := exec.Command("go", "test", "-race", "./...")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOWORK=off", "GOTOOLCHAIN=go1.27.1")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	for _, tc := range []struct{ name, source, want string }{
		{"missing constructor", strings.Replace(original, "func NewContext() *Context { return &Context{cache: make(map[int]int)} }", "", 1), "undefined: NewContext"},
		{"dependent constructor", strings.Replace(original, "return &Context{cache: make(map[int]int)}", "_ = table; return &Context{cache: make(map[int]int)}", 1), "depends on state"},
		{"field collision", strings.Replace(original, "mu sync.Mutex", "answer int; mu sync.Mutex", 1), "field collision"},
		{"wrong result", strings.Replace(strings.Replace(original, "func NewContext() *Context { return &Context{cache: make(map[int]int)} }", "func NewContext() {}", 1), "func(c *Context) Spawn()*Context{return NewContext()}", "", 1), "must have signature"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := render(t, tc.source, plan)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("got %v; want %s", err, tc.want)
			}
		})
	}
}

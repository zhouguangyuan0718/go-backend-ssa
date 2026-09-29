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

// Native value constructors have input parameters and must keep imports from
// their own source file. Constructor dependencies are checked transitively.
func TestNativeValueContext(t *testing.T) {
	const ctor = `package p
 import "bytes"
 type Context struct{ cache string }
 func pure(s string)string { return s+"!" }
 func NewContext(b *bytes.Buffer, suffix ...string)Context{return Context{cache:pure(b.String()+suffix[0])}}
 func(c *Context) Text()string{return c.cache}
 `
	const state = `package p
 var count=7
 func(c *Context) Next()int{count++;return count}
 `
	contextRule := func(fields map[string]string) *ContextRule {
		return &ContextRule{Existing: true, Selective: true, Type: "Context", Constructor: "NewContext", Fields: fields}
	}
	plan := Plan{InspectRoots: []string{"cmd/compile/internal/p"}, Units: []Unit{
		{Source: "src/cmd/compile/internal/p/a.go", WholeFile: true, Context: contextRule(map[string]string{"count": "count"})},
		{Source: "src/cmd/compile/internal/p/z.go", WholeFile: true, Context: contextRule(nil)},
	}}
	repo, pin := fixtureRepo(t, map[string]string{"LICENSE": "fixture", "PATENTS": "fixture", plan.Units[0].Source: state, plan.Units[1].Source: ctor})
	s, err := OpenSource(context.Background(), repo, pin)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	r, err := Render(s, plan, dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := Install(dir, r); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, dir, "go.mod", "module go-backend-ssa\n\ngo 1.27.1\n")
	writeTestFile(t, dir, "internal/p/context_test.go", `package p
 import("testing";"bytes")
 func TestConstructor(t *testing.T){a:=NewContext(bytes.NewBufferString("a"),"b");b:=NewContext(bytes.NewBufferString("b"),"c");if a.Text()!="ab!"||b.Text()!="bc!"||a.Next()!=8||a.Next()!=9||b.Next()!=8{t.Fatal("constructor parameters or state")}}
 `)
	cmd := exec.Command("go", "test", "-race", "./...")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOWORK=off", "GOTOOLCHAIN=go1.27.1")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	s.cache[plan.Units[1].Source] = []byte(strings.Replace(ctor, `return s+"!"`, `c:=Context{};c.Next();return s+"!"`, 1))
	if _, err := Render(s, plan, dir); err == nil || !strings.Contains(err.Error(), "depends on state") {
		t.Fatalf("indirect constructor dependency: %v", err)
	}
	s.cache[plan.Units[1].Source] = []byte(ctor)
	s.cache[plan.Units[0].Source] = []byte(state + `var saved=&count`)
	plan.Units[0].Context.Fields["saved"] = "saved"
	if _, err := Render(s, plan, dir); err == nil || !strings.Contains(err.Error(), "initializer may retain") {
		t.Fatalf("value copy back-pointer: %v", err)
	}
}

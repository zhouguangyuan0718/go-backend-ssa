package port

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestIndependentContextInitializers(t *testing.T) {
	const a = `package p
var table=[]int{3}
func Base()int{return table[0]}
func init(){table[0]+=7}
`
	const b = `package p
var result=seed+1
var callback=Base
var checks []int
var singleton Node
type Node struct{}
func (n *Node)Read()int{return result}
func init(){checks=append(checks,singleton.Read())}
`
	const c = `package p
var seed=5
func init(){checks=append(checks,seed)}
`
	const root = "src/cmd/compile/internal/p/"
	repo, pin := fixtureRepo(t, map[string]string{"LICENSE": "fixture", "PATENTS": "fixture", root + "a.go": a, root + "b.go": b, root + "c.go": c})
	source, err := OpenSource(context.Background(), repo, pin)
	if err != nil {
		t.Fatal(err)
	}
	ctx := func(group string, fields ...string) *ContextRule {
		r := &ContextRule{Type: "Context", Constructor: "initialize", Initialize: true, Adapter: "adapter_context.go", Selective: true, Init: true, InitGroup: group, Fields: map[string]string{}}
		for _, field := range fields {
			r.Fields[field] = field
		}
		return r
	}
	// Deliberately reverse the migration list. Each group's variable dependency
	// order and lexical init-body order must still be native Go order.
	plan := Plan{InspectRoots: []string{"cmd/compile/internal/p"}, Units: []Unit{
		{Source: root + "c.go", WholeFile: true, Context: ctx("initializePhases", "seed")},
		{Source: root + "b.go", WholeFile: true, Context: ctx("initializePhases", "result", "callback", "checks", "singleton"), Owner: &OwnerRule{Types: []string{"Node"}, Context: "Context", Field: "owner"}},
		{Source: root + "a.go", WholeFile: true, Context: ctx("", "table")},
	}}
	dir := t.TempDir()
	writeTestFile(t, dir, "internal/p/adapter_context.go", `package p;type Context struct{table,checks []int;seed,result int;callback func()int;singleton Node}`)
	rendered, err := Render(source, plan, dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := Install(dir, rendered); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, dir, "go.mod", "module go-backend-ssa\n\ngo 1.27.1\n")
	writeTestFile(t, dir, "internal/p/check_test.go", `package p
import("testing";"sync")
func TestGroups(t *testing.T){var wg sync.WaitGroup;for range 8{wg.Go(func(){c:=new(Context);c.initialize();if len(c.table)!=1||c.table[0]!=10||c.checks!=nil||c.callback!=nil||c.seed!=0||c.singleton.owner!=nil{t.Error("late state initialized early")};c.initializePhases();if c.seed!=5||c.result!=6||len(c.checks)!=2||c.checks[0]!=6||c.checks[1]!=5||c.singleton.owner!=c||c.callback()!=10{t.Errorf("lost native initialization: %+v",c)};c.table[0]=30;if c.callback()!=30{t.Error("callback did not retain instance")}})};wg.Wait()}
`)
	cmd := exec.Command("go", "test", "-race", "./...")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOWORK=off", "GOTOOLCHAIN=go1.27.1")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	for name, change := range map[string]struct{ file, code string }{
		"early reads late":       {"a.go", strings.Replace(a, "[]int{3}", "[]int{seed}", 1)},
		"late reads early":       {"b.go", strings.Replace(b, "seed+1", "seed+Base()", 1)},
		"early init reads late":  {"a.go", strings.Replace(a, "+=7", "+=result", 1)},
		"early init writes late": {"a.go", a + `func init(){seed=7}`},
		"late init reads early":  {"b.go", strings.Replace(b, "checks,singleton.Read()", "checks,Base()", 1)},
		"indirect helper":        {"a.go", strings.Replace(a, "[]int{3}", "[]int{Helper()}", 1) + `func Helper()int{return More()};func More()int{return seed}`},
		"receiver helper":        {"a.go", strings.Replace(a, "[]int{3}", "[]int{singleton.Read()}", 1)},
		"immediate closure":      {"a.go", strings.Replace(a, "[]int{3}", "[]int{func()int{return seed}()}", 1)},
		"callback argument":      {"a.go", strings.Replace(a, "[]int{3}", "[]int{Invoke(func()int{return seed})}", 1) + `func Invoke(f func()int)int{return f()}`},
		"indirect call":          {"a.go", a + `func init(){_ = callback()}`},
		"bodyless local call":    {"a.go", strings.Replace(a, "[]int{3}", "[]int{Native()}", 1) + `func Native()int`},
		"interface call":         {"a.go", strings.Replace(a, "[]int{3}", "[]int{ReadInterface()}", 1) + `type Reader struct{};func(Reader)Read()int{return seed};func ReadInterface()int{var v interface{Read()int}=Reader{};return v.Read()}`},
		"group method collision": {"b.go", b + `func initializePhases(){}`},
	} {
		t.Run(name, func(t *testing.T) {
			path := root + change.file
			old := source.cache[path]
			source.cache[path] = []byte(change.code)
			defer func() { source.cache[path] = old }()
			if _, err := Render(source, plan, dir); err == nil || !strings.Contains(err.Error(), "initializer group") {
				t.Fatalf("unsafe group accepted or unexpected failure: %v", err)
			}
		})
	}
	for _, group := range []string{"initialize", "_", "not-valid"} {
		old := plan.Units[0].Context.InitGroup
		plan.Units[0].Context.InitGroup = group
		if err := plan.Validate(); err == nil {
			t.Fatalf("invalid group accepted: %s", group)
		}
		plan.Units[0].Context.InitGroup = old
	}
	plan.Units[0].Context.Initialize = false
	if err := plan.Validate(); err == nil {
		t.Fatal("group without receiver initialization accepted")
	}
}

func TestIndependentGenericInitializers(t *testing.T) {
	const root = "src/cmd/compile/internal/p/"
	const a = `package p
 import "reflect"
 var nodeType=reflect.TypeFor[int]()
 var values=[]int{identity[int](3),pair[int,int](1,2)}
 func identity[T any](v T)T{return v}
 func pair[A ~int,B ~int](a A,b B)int{return int(a)+int(b)}
 `
	const b = `package p;var late=7`
	repo, pin := fixtureRepo(t, map[string]string{"LICENSE": "fixture", "PATENTS": "fixture", root + "a.go": a, root + "b.go": b})
	source, e := OpenSource(context.Background(), repo, pin)
	if e != nil {
		t.Fatal(e)
	}
	ctx := func(group string, fields ...string) *ContextRule {
		c := &ContextRule{Type: "Context", Constructor: "initialize", Initialize: true, Selective: true, Adapter: "adapter_context.go", InitGroup: group, Fields: map[string]string{}}
		for _, f := range fields {
			c.Fields[f] = f
		}
		return c
	}
	plan := Plan{InspectRoots: []string{"cmd/compile/internal/p"}, Units: []Unit{{Source: root + "a.go", WholeFile: true, Context: ctx("", "nodeType", "values"), StatelessFunctions: []string{"identity", "pair"}}, {Source: root + "b.go", WholeFile: true, Context: ctx("initializeLate", "late")}}}
	dir := t.TempDir()
	writeTestFile(t, dir, "internal/p/adapter_context.go", `package p;import "reflect";type Context struct{nodeType reflect.Type;values []int;late int}`)
	r, e := Render(source, plan, dir)
	if e != nil {
		t.Fatal(e)
	}
	if e = Install(dir, r); e != nil {
		t.Fatal(e)
	}
	writeTestFile(t, dir, "go.mod", "module go-backend-ssa\n\ngo 1.27.1\n")
	writeTestFile(t, dir, "internal/p/check_test.go", `package p;import("reflect";"testing");func TestGeneric(t *testing.T){c:=new(Context);c.initialize();if c.nodeType!=reflect.TypeFor[int]()||c.values[0]!=3||c.values[1]!=3||c.late!=0{t.Fatal("wrong early state")};c.initializeLate();if c.late!=7{t.Fatal("wrong late state")}}`)
	cmd := exec.Command("go", "test", "-race", "./...")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOWORK=off", "GOTOOLCHAIN=go1.27.1")
	if out, e := cmd.CombinedOutput(); e != nil {
		t.Fatalf("%v\n%s", e, out)
	}
	for name, code := range map[string]string{
		"generic helper reads late": strings.Replace(a, "return int(a)+int(b)", "return late+int(a)+int(b)", 1),
		"indexed callback":          a + `var callbacks=[]func()int{func()int{return 1}};var unresolved=callbacks[0]()`,
	} {
		t.Run(name, func(t *testing.T) {
			source.cache[root+"a.go"] = []byte(code)
			if name == "indexed callback" {
				defer delete(plan.Units[0].Context.Fields, "callbacks")
				defer delete(plan.Units[0].Context.Fields, "unresolved")
				plan.Units[0].Context.Fields["callbacks"] = "callbacks"
				plan.Units[0].Context.Fields["unresolved"] = "unresolved"
			}
			if _, e := Render(source, plan, dir); e == nil || !strings.Contains(e.Error(), "initializer group") {
				t.Fatalf("unreviewed initialization accepted or wrong failure: %v", e)
			}
		})
	}
}

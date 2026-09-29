package port

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestSelectiveContextFunctions(t *testing.T) {
	const code = `package p
 type Context struct{cache map[int]int;Read int}
 func NewContext()*Context{return &Context{cache:make(map[int]int)}}
 type Kind int
 func(k Kind)String()string{return label(int(k))}
 func label(n int)string{if pureIdentity(n)==1{return "one"};return "other"}
 func pureIdentity[T any](n T)T{return n}
 type list []int
 func(l list)Sum()int{var n int;for _,x:=range l{n+=pureIdentity(x)};return n}
 var counter int
 var callback=Forward
 func Read()int{counter++;return counter}
 func Forward()int{return Read()}
 func Closure()func()int{return func()int{return Forward()}}
 func(c *Context)Value()int{return callback()}
 func Shadow()int{Read:=func()int{return 9};return Read()}
 func Field(c *Context)int{return c.Read}
 `
	repo, pin := fixtureRepo(t, map[string]string{"LICENSE": "fixture", "PATENTS": "fixture", "src/cmd/compile/internal/p/p.go": code})
	s, err := OpenSource(context.Background(), repo, pin)
	if err != nil {
		t.Fatal(err)
	}
	plan := Plan{InspectRoots: []string{"cmd/compile/internal/p"}, Units: []Unit{{Source: "src/cmd/compile/internal/p/p.go", WholeFile: true, ContextMethods: map[string]string{"Read": "nextRead"}, Context: &ContextRule{Existing: true, Selective: true, Type: "Context", Constructor: "NewContext", Fields: map[string]string{"counter": "counter", "callback": "callback"}}}}}
	dir := t.TempDir()
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
 func TestBindings(t *testing.T){a,b:=NewContext(),NewContext();if label(1)!="one"||Kind(2).String()!="other"||(list{1,2}).Sum()!=3||Shadow()!=9{t.Fatal("pure helper changed")};a.Read=99;if Field(a)!=99||a.Value()!=1||a.Closure()()!=2||a.Forward()!=3||b.Value()!=1||a.nextRead()!=4{t.Fatal("context transitive binding")};var wg sync.WaitGroup;for range 12{wg.Go(func(){c:=NewContext();if c.Value()!=1{t.Error("shared callback")}})};wg.Wait()}
 `)
	cmd := exec.Command("go", "test", "-race", "./...")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOWORK=off", "GOTOOLCHAIN=go1.27.1")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	for _, tc := range []struct{ source, want string }{{"absent", "not selected"}, {"label", "does not need context"}} {
		p := plan
		p.Units = append([]Unit(nil), plan.Units...)
		p.Units[0].ContextMethods = map[string]string{tc.source: "renamed"}
		if _, err := Render(s, p, t.TempDir()); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Fatalf("got %v, want %s", err, tc.want)
		}
	}

}

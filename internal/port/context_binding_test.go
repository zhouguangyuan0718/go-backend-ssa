package port

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestContextMethodValueBinding(t *testing.T) {
	const source = `package p
 type Context struct{}
 func NewContext()*Context{return &Context{}}
 var calls int
 type counter struct{n int}
 func(c *counter)next(delta ...int)int{if c==nil{return calls};for _,x:=range delta{c.n+=x};calls++;return c.n}
 func(c counter)snapshot()int{return c.n+calls}
 func receiver()*counter{calls++;return &counter{n:calls}}
 func Factory()(func(...int)int,func(*counter,...int)int){return receiver().next,(*counter).next}
 func Nil()func(...int)int{return (*counter)(nil).next}
 func Address()(func(...int)int,func()int){c:=counter{1};ptr,value:=c.next,c.snapshot;c.n=7;return ptr,value}
 func Deref(){var p *counter;_ = p.snapshot}
 func Forward()any{return receiver().foreign}
 func(c *counter)recoverPanic(out *any){calls++;*out=recover()}
 func Catch()(out any){c:=&counter{};defer c.recoverPanic(&out);panic("original panic")}
 `
	const extra = `package p
 import val "go/constant"
 func(c *counter)foreign(v val.Value)val.Value{calls++;return v}
 `
	repo, pin := fixtureRepo(t, map[string]string{"LICENSE": "fixture", "PATENTS": "fixture", "src/cmd/compile/internal/p/a.go": source, "src/cmd/compile/internal/p/b.go": extra})
	s, err := OpenSource(context.Background(), repo, pin)
	if err != nil {
		t.Fatal(err)
	}
	plan := Plan{InspectRoots: []string{"cmd/compile/internal/p"}, Units: []Unit{
		{Source: "src/cmd/compile/internal/p/a.go", WholeFile: true, ContextParams: []string{"counter.next", "counter.snapshot", "counter.recoverPanic"}, Context: &ContextRule{Existing: true, Selective: true, Type: "Context", Constructor: "NewContext", Fields: map[string]string{"calls": "calls"}}},
		{Source: "src/cmd/compile/internal/p/b.go", WholeFile: true, ContextParams: []string{"counter.foreign"}, Context: &ContextRule{Existing: true, Selective: true, Type: "Context", Constructor: "NewContext"}},
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
	writeTestFile(t, dir, "internal/p/adapter_test.go", `package p
 import("testing";"go/constant";"sync")
 func TestBindings(t *testing.T){a,b:=NewContext(),NewContext();if b.Catch()!="original panic"{t.Fatal("direct deferred recover changed")};b=NewContext();bound,expr:=a.Factory();if a.calls!=1||bound(3)!=4||bound(2)!=6||a.calls!=3{t.Fatal("receiver reevaluated")};c:=&counter{n:10};if expr(c,1,2)!=13||a.calls!=4||a.Nil()()!=4||b.Nil()()!=0{t.Fatal("expression/variadic/nil")};ptr,val:=a.Address();if ptr()!=7||val()!=6{t.Fatal("pointer versus value capture")};f:=a.Forward().(func(constant.Value)constant.Value);v:=constant.MakeInt64(7);if f(v)!=v{t.Fatal("imported signature")};func(){defer func(){if recover()==nil{t.Error("nil value receiver must panic at capture")}}();a.Deref()}();var wg sync.WaitGroup;for range 12{wg.Go(func(){c:=NewContext();f,_:=c.Factory();if f(1)!=2||c.Nil()()!=2{t.Error("callback shared state")}})};wg.Wait()}
 `)
	cmd := exec.Command("go", "test", "-race", "./...")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOWORK=off", "GOTOOLCHAIN=go1.27.1")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	s.cache[plan.Units[0].Source] = []byte(source + "\nfunc Invalid()any{return (&counter{}).recoverPanic}\n")
	if _, err := Render(s, plan, t.TempDir()); err == nil || !strings.Contains(err.Error(), "direct recover") {
		t.Fatalf("unsafe recover binding accepted: %v", err)
	}

}

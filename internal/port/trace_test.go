package port

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestInstanceTrace(t *testing.T) {
	const source = `package p
 import("fmt";"os";"runtime";"log")
 type Context struct{}
 func NewContext()*Context{return &Context{}}
 func Emit(){fmt.Printf("%s", "hello");fmt.Println(" world");fmt.Fprint(os.Stderr,"error\n");f:=fmt.Printf;f("callback\n")}
 func Caller()bool{_,_,_,ok:=runtime.Caller(0);return ok}
 func Fail(){log.Panicf("invalid pass %d",7)}
 `
	const adapter = `package p
 import("fmt";"io")
 type contextIO struct{TraceWriter io.Writer}
 type discardTrace struct{}
 func(discardTrace)Write(p []byte)(int,error){return len(p),nil}
 func(c *Context)traceWriter()io.Writer{if c.TraceWriter!=nil{return c.TraceWriter};return discardTrace{}}
 func(c *Context)tracePrintf(f string,args ...any)(int,error){return fmt.Fprintf(c.traceWriter(),f,args...)}
 func(c *Context)tracePrintln(args ...any)(int,error){return fmt.Fprintln(c.traceWriter(),args...)}
 func(c *Context)panicLogf(f string,args ...any){s:=fmt.Sprintf(f,args...);fmt.Fprintln(c.traceWriter(),s);panic(s)}
 `
	repo, pin := fixtureRepo(t, map[string]string{"LICENSE": "fixture", "PATENTS": "fixture", "src/cmd/compile/internal/p/p.go": source})
	s, err := OpenSource(context.Background(), repo, pin)
	if err != nil {
		t.Fatal(err)
	}
	plan := Plan{InspectRoots: []string{"cmd/compile/internal/p"}, Units: []Unit{{Source: "src/cmd/compile/internal/p/p.go", WholeFile: true, Context: &ContextRule{Existing: true, Selective: true, Type: "Context", Constructor: "NewContext", Adapter: "adapter_context.go", Embed: "contextIO", RedirectTrace: true}}}}
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
	writeTestFile(t, dir, "internal/p/adapter_test.go", `package p
 import("testing";"bytes";"sync")
 func TestTrace(t *testing.T){NewContext().Emit();if !Caller(){t.Fatal("stack lookup")};var wg sync.WaitGroup;for range 16{wg.Go(func(){c:=NewContext();var b bytes.Buffer;c.TraceWriter=&b;c.Emit();if b.String()!="hello world\nerror\ncallback\n"{t.Error(b.String())}})};wg.Wait()}
 func TestPanic(t *testing.T){c:=NewContext();var b bytes.Buffer;c.TraceWriter=&b;defer func(){if got:=recover();got!="invalid pass 7"||b.String()!="invalid pass 7\n"{t.Errorf("panic=%v trace=%q",got,b.String())}}();c.Fail();t.Fatal("panic returned")}
 `)
	cmd := exec.Command("go", "test", "-race", "./...")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOWORK=off", "GOTOOLCHAIN=go1.27.1")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	for _, extra := range []string{"func Bad(){os.Exit(1)}", "func Bad(){runtime.GOMAXPROCS(1)}"} {
		s.cache[plan.Units[0].Source] = []byte(source + extra)
		if _, err := Render(s, plan, dir); err == nil || !strings.Contains(err.Error(), "unclassified environment") {
			t.Fatalf("ambient operation audit: %v", err)
		}
	}
	s.cache[plan.Units[0].Source] = []byte(source)
	writeTestFile(t, dir, "internal/p/adapter_context.go", strings.Replace(adapter, "(int,error){return fmt.Fprintf(c.traceWriter(),f,args...)}", "(int,int){return 0,0}", 1))
	if _, err := Render(s, plan, dir); err == nil || !strings.Contains(err.Error(), "signature mismatch") {
		t.Fatalf("invalid trace signature accepted: %v", err)
	}
}

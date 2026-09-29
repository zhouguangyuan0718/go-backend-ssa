package port

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
)

// Link-like native contexts are often created as zero values by a host. Their
// original fields and identity must survive installation of per-instance state.
func TestNativeReceiverInitializerAndFatalAdapters(t *testing.T) {
	const source = `package p
 import "log"
 type Link struct{ Host int }
 var cache=map[int]int{}
 func Set(k,v int){cache[k]=v}
 func(c *Link)Get(k int)int{return c.Host+cache[k]}
 func Fail(n int){if n==1{log.Fatal("one")};log.Fatalf("bad %d",n)}
 `
	repo, pin := fixtureRepo(t, map[string]string{"LICENSE": "fixture", "PATENTS": "fixture", "src/cmd/internal/p/p.go": source})
	s, e := OpenSource(context.Background(), repo, pin)
	if e != nil {
		t.Fatal(e)
	}
	c := &ContextRule{Existing: true, Initialize: true, Selective: true, RedirectExit: true, Type: "Link", Constructor: "initialize", Adapter: "adapter_context.go", Fields: map[string]string{"cache": "cache"}}
	plan := Plan{InspectRoots: []string{"cmd/internal/p"}, Units: []Unit{{Source: "src/cmd/internal/p/p.go", WholeFile: true, Context: c}}}
	dir := t.TempDir()
	writeTestFile(t, dir, "internal/cmd/p/adapter_context.go", `package p
 import "fmt"
 func(l *Link)fatalLog(a ...any){panic(fmt.Sprint(a...))}
 func(l *Link)fatalLogf(f string,a ...any){panic(fmt.Sprintf(f,a...))}
 `)
	r, e := Render(s, plan, dir)
	if e != nil {
		t.Fatal(e)
	}
	if e = Install(dir, r); e != nil {
		t.Fatal(e)
	}
	writeTestFile(t, dir, "go.mod", "module go-backend-ssa\n\ngo 1.27.1\n")
	writeTestFile(t, dir, "internal/cmd/p/check_test.go", `package p
 import("testing";"sync")
 func TestNativeInitialization(t *testing.T){var wg sync.WaitGroup;for i:=range 12{wg.Add(1);go func(){defer wg.Done();l:=&Link{Host:i};same:=l;l.initialize();l.Set(1,i);if same!=l||l.Get(1)!=2*i{t.Error("native state or identity")};for _,n:=range []int{1,2}{func(){defer func(){if recover()==nil{t.Error("host not aborted")}}();l.Fail(n)}()}}()};wg.Wait()}
 `)
	cmd := exec.Command("go", "test", "-race", "./...")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOWORK=off", "GOTOOLCHAIN=go1.27.1")
	if out, e := cmd.CombinedOutput(); e != nil {
		t.Fatalf("%v\n%s", e, out)
	}
	s.cache[plan.Units[0].Source] = []byte(source + `func initialize()*Link{return &Link{}}`)
	if _, e = Render(s, plan, dir); e == nil || !strings.Contains(e.Error(), "cannot wrap a native constructor") {
		t.Fatalf("constructor not rejected: %v", e)
	}
}

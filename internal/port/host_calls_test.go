package port

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestBoundCompilerHostCalls(t *testing.T) {
	const source = `package p
import("os";"io";"runtime";"time")
func Output(name string)(io.WriteCloser,error){return os.Create(os.Getenv("GOSSADIR")+name)}
func Stats()(uint64,uint64,int64){var a,b runtime.MemStats;runtime.ReadMemStats(&a);start:=time.Now();runtime.ReadMemStats(&b);end:=time.Now();return b.TotalAlloc-a.TotalAlloc,b.Mallocs-a.Mallocs,end.Sub(start).Nanoseconds()}
`
	const adapter = `package p
import("io";"runtime";"time")
type Context struct{Directory string;Create func(string)(io.WriteCloser,error);Metric func(*runtime.MemStats);Clock func()time.Time}
func(c *Context)environment(key string)string{if key!="GOSSADIR"{panic(key)};return c.Directory}
func(c *Context)create(name string)(io.WriteCloser,error){return c.Create(name)}
func(c *Context)metric(m *runtime.MemStats){c.Metric(m)}
func(c *Context)clock()time.Time{return c.Clock()}
`
	const path = "src/cmd/compile/internal/p/p.go"
	repo, pin := fixtureRepo(t, map[string]string{"LICENSE": "fixture", "PATENTS": "fixture", path: source})
	s, err := OpenSource(context.Background(), repo, pin)
	if err != nil {
		t.Fatal(err)
	}
	unit := Unit{Source: path, WholeFile: true, Context: &ContextRule{Type: "Context", Constructor: "initialize", Initialize: true, Selective: true, Adapter: "adapter_context.go"}, HostCalls: []HostCallRule{{Package: "os", Function: "Getenv", Method: "environment", StringArgument: "GOSSADIR"}, {Package: "os", Function: "Create", Method: "create"}, {Package: "runtime", Function: "ReadMemStats", Method: "metric"}, {Package: "time", Function: "Now", Method: "clock"}}}
	plan := Plan{InspectRoots: []string{"cmd/compile/internal/p"}, Units: []Unit{unit}}
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
	writeTestFile(t, dir, "internal/p/check_test.go", `package p
import("bytes";"errors";"fmt";"io";"runtime";"sync";"testing";"time")
type buffer struct{bytes.Buffer;closed bool};func(b *buffer)Close()error{b.closed=true;return nil}
func TestProviders(t *testing.T){var wg sync.WaitGroup;for i:=range 8{wg.Go(func(){var output buffer;metric,clock:=0,0;c:=&Context{Directory:fmt.Sprint(i,"/"),Create:func(name string)(io.WriteCloser,error){if name!=fmt.Sprint(i,"/result"){t.Error(name)};return &output,nil},Metric:func(m *runtime.MemStats){metric++;m.TotalAlloc=uint64(metric*(i+1));m.Mallocs=uint64(metric*2)},Clock:func()time.Time{clock++;return time.Unix(0,int64(clock*7))}};w,err:=c.Output("result");if err!=nil{t.Error(err)};fmt.Fprint(w,i);w.Close();if !output.closed||output.String()!=fmt.Sprint(i){t.Error("writer flow")};a,b,n:=c.Stats();if a!=uint64(i+1)||b!=2||n!=7||metric!=2||clock!=2{t.Error(a,b,n)};failure:=errors.New("no output");c.Create=func(string)(io.WriteCloser,error){return nil,failure};if w,err:=c.Output("result");w!=nil||err!=failure{t.Error("error flow")}})};wg.Wait()}
`)
	cmd := exec.Command("go", "test", "-race", "./...")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOWORK=off", "GOTOOLCHAIN=go1.27.1")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	for name, changed := range map[string]string{
		"wrong environment key":   strings.Replace(source, "GOSSADIR", "HOME", 1),
		"dynamic environment key": strings.Replace(source, `os.Getenv("GOSSADIR")`, `os.Getenv(name)`, 1),
		"environment callback":    strings.Replace(source, `return os.Create`, `lookup:=os.Getenv;_ = lookup;return os.Create`, 1),
		"ambient operation":       source + `func Bad(){os.Remove("x")}`,
		"runtime operation":       source + `func Bad(){runtime.GC()}`,
		"unreviewed metric field": strings.Replace(source, "b.Mallocs-a.Mallocs", "uint64(b.NumGC-a.NumGC)", 1),
		"metric literal":          source + `func Bad(){_ = runtime.MemStats{}}`,
		"foreign metric field":    source + `func Bad(m *runtime.MemStats)uint64{return m.TotalAlloc}`,
	} {
		t.Run(name, func(t *testing.T) {
			s.cache[path] = []byte(changed)
			defer func() { s.cache[path] = []byte(source) }()
			_, err := Render(s, plan, dir)
			if err == nil {
				t.Fatal("unreviewed host operation accepted")
			}
			if name == "unreviewed metric field" && !strings.Contains(err.Error(), "unclassified environment dependency runtime.NumGC") {
				t.Fatalf("metric rejection did not reach the output audit: %v", err)
			}
		})
	}
	for name, changed := range map[string]string{
		"missing close":      strings.ReplaceAll(adapter, "io.WriteCloser", "io.Writer"),
		"native file escape": strings.ReplaceAll(strings.Replace(adapter, `"io"`, `"os"`, 1), "io.WriteCloser", "*os.File"),
		"wrong clock result": strings.ReplaceAll(strings.ReplaceAll(adapter, "func()time.Time", "func()int64"), "clock()time.Time", "clock()int64"),
	} {
		t.Run(name, func(t *testing.T) {
			writeTestFile(t, dir, "internal/p/adapter_context.go", changed)
			defer writeTestFile(t, dir, "internal/p/adapter_context.go", adapter)
			if _, err := Render(s, plan, dir); err == nil {
				t.Fatal("invalid host adapter accepted")
			}
		})
	}
	for _, r := range []HostCallRule{{Package: "os", Function: "Remove", Method: "remove"}, {Package: "os", Function: "Getenv", Method: "env"}, {Package: "time", Function: "Now", Method: "now", StringArgument: "unused"}, {Package: "time", Function: "Now", Method: "_"}} {
		p := plan
		p.Units = append([]Unit(nil), plan.Units...)
		p.Units[0].HostCalls = []HostCallRule{r}
		if err := p.Validate(); err == nil {
			t.Fatal("invalid host rule accepted")
		}
	}
}

func TestBoundQueuePermutation(t *testing.T) {
	const path = "src/cmd/compile/internal/p/p.go"
	const source = `package p; import "math/rand";func Order(n int)[]int{return rand.Perm(n)}`
	const adapter = `package p;import "math/rand";type Context struct{R *rand.Rand};func(c *Context)perm(n int)[]int{return c.R.Perm(n)}`
	repo, pin := fixtureRepo(t, map[string]string{"LICENSE": "fixture", "PATENTS": "fixture", path: source})
	s, err := OpenSource(context.Background(), repo, pin)
	if err != nil {
		t.Fatal(err)
	}
	unit := Unit{Source: path, WholeFile: true, Context: &ContextRule{Type: "Context", Constructor: "initialize", Initialize: true, Selective: true, Adapter: "adapter_context.go"}, HostCalls: []HostCallRule{{Package: "math/rand", Function: "Perm", Method: "perm"}}}
	plan := Plan{InspectRoots: []string{"cmd/compile/internal/p"}, Units: []Unit{unit}}
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
	writeTestFile(t, dir, "internal/p/check_test.go", `package p;import("math/rand";"slices";"testing");func TestOrder(t *testing.T){for i:=range 8{t.Run(string(rune('a'+i)),func(t *testing.T){t.Parallel();a:=&Context{R:rand.New(rand.NewSource(int64(i)))};b:=&Context{R:rand.New(rand.NewSource(int64(i)))};want:=a.Order(31);saved:=slices.Clone(want);_ =a.Order(71);if !slices.Equal(want,saved)||!slices.Equal(b.Order(31),saved){t.Fatal("random stream or retained permutation shared")}})}}`)
	cmd := exec.Command("go", "test", "-race", "./...")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOWORK=off", "GOTOOLCHAIN=go1.27.1")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	writeTestFile(t, dir, "internal/p/adapter_context.go", strings.Replace(adapter, "perm(n int)[]int", "perm(n int)[]int64", 1))
	if _, err := Render(s, plan, dir); err == nil {
		t.Fatal("incorrect permutation result accepted")
	}
}

package port

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestNativeDependencyHostInputs(t *testing.T) {
	const path = "src/cmd/compile/internal/p/p.go"
	const source = `package p
import("os";"runtime")
func Open(name string)(*os.File,error){if runtime.GOOS=="windows"{name="windows/"+name};return os.Open(name)}
`
	const adapter = `package p
import "io"
type Stream interface{io.Reader;io.ReaderAt;io.Seeker;io.Closer;Name()string}
type Context struct{HostOS string;OpenStream func(string)(Stream,error)}
func(c *Context)open(name string)(Stream,error){return c.OpenStream(name)}
`
	repo, pin := fixtureRepo(t, map[string]string{"LICENSE": "fixture", "PATENTS": "fixture", path: source})
	s, e := OpenSource(context.Background(), repo, pin)
	if e != nil {
		t.Fatal(e)
	}
	methods := []string{"Read", "ReadAt", "Seek", "Close", "Name"}
	plan := Plan{InspectRoots: []string{"cmd/compile/internal/p"}, Units: []Unit{{Source: path, WholeFile: true, Context: &ContextRule{Type: "Context", Constructor: "initialize", Initialize: true, Selective: true, Adapter: "adapter_context.go"}, HostSignatures: []HostSignatureRule{{Function: "Open", Result: true, Methods: methods}}, HostConstants: []HostConstantRule{{Package: "runtime", Constant: "GOOS", Field: "HostOS"}}, HostCalls: []HostCallRule{{Package: "os", Function: "Open", Method: "open", StreamMethods: methods}}}}}
	dir := t.TempDir()
	writeTestFile(t, dir, "internal/p/adapter_context.go", adapter)
	r, e := Render(s, plan, dir)
	if e != nil {
		t.Fatal(e)
	}
	if e = Install(dir, r); e != nil {
		t.Fatal(e)
	}
	recorded := 0
	for _, entry := range r.Ledger.Files {
		for _, decl := range entry.Declarations {
			if strings.HasPrefix(decl, "host-constant:") || strings.HasPrefix(decl, "host-factory-stream:") {
				recorded++
			}
		}
	}
	if recorded != 2 {
		t.Fatal("missing input provenance", recorded)
	}
	writeTestFile(t, dir, "go.mod", "module go-backend-ssa\n\ngo 1.27.1\n")
	writeTestFile(t, dir, "internal/p/check_test.go", `package p
import("bytes";"errors";"sync";"testing")
type stream struct{*bytes.Reader;name string};func(s *stream)Name()string{return s.name};func(s *stream)Close()error{return nil}
func TestInputs(t *testing.T){var wg sync.WaitGroup;for i:=range 8{wg.Go(func(){host:="linux";name:="file";if i%2==0{host="windows";name="windows/file"};original:=&stream{bytes.NewReader([]byte("abc")),name};c:=&Context{HostOS:host,OpenStream:func(got string)(Stream,error){if got!=name{t.Error(got,name)};return original,nil}};f,e:=c.Open("file");if f!=original||e!=nil||f.Name()!=name{t.Error(f,e)};failure:=errors.New("open");c.OpenStream=func(string)(Stream,error){return nil,failure};if f,e=c.Open("file");f!=nil||e!=failure{t.Error(f,e)}})};wg.Wait()}
`)
	cmd := exec.Command("go", "test", "-race", "./...")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOWORK=off", "GOTOOLCHAIN=go1.27.1")
	if b, e := cmd.CombinedOutput(); e != nil {
		t.Fatalf("%v\n%s", e, b)
	}
	for name, changed := range map[string]string{"extra capability": strings.Replace(adapter, "Name()string", "Name()string;Sync()error", 1), "missing capability": strings.Replace(adapter, "io.ReaderAt;", "", 1), "wrong host type": strings.Replace(adapter, "HostOS string", "HostOS int", 1)} {
		t.Run(name, func(t *testing.T) {
			writeTestFile(t, dir, "internal/p/adapter_context.go", changed)
			defer writeTestFile(t, dir, "internal/p/adapter_context.go", adapter)
			if _, e := Render(s, plan, dir); e == nil {
				t.Fatal("invalid host adapter accepted")
			}
		})
	}
	for name, changed := range map[string]string{"unused host input": strings.Replace(source, `runtime.GOOS=="windows"`, `false`, 1), "unreviewed host constant": source + `;func Bad()string{return runtime.GOARCH}`, "unreviewed file operation": source + `;func Bad(f *os.File)error{return f.Sync()}`} {
		t.Run(name, func(t *testing.T) {
			s.cache[path] = []byte(changed)
			defer func() { s.cache[path] = []byte(source) }()
			if _, e := Render(s, plan, dir); e == nil {
				t.Fatal("invalid host source accepted")
			}
		})
	}
	for _, rule := range []HostCallRule{{Package: "os", Function: "Create", Method: "open", StreamMethods: methods}, {Package: "os", Function: "Open", Method: "open", StreamMethods: []string{"Read", "Close", "Fd"}}, {Package: "os", Function: "Open", Method: "open", StreamMethods: []string{"Read"}}, {Package: "os", Function: "Open", Method: "open", StreamMethods: []string{"Read", "Close", "Read"}}} {
		p := plan
		p.Units = append([]Unit(nil), plan.Units...)
		p.Units[0].HostCalls = []HostCallRule{rule}
		if p.Validate() == nil {
			t.Fatal("invalid factory rule accepted", rule)
		}
	}
}
func TestExplicitNativeBuildVariant(t *testing.T) {
	const path = "src/cmd/compile/internal/p/p_read.go"
	const source = "//go:build !unix\n\npackage p\nfunc F()int{return 7}\n"
	repo, pin := fixtureRepo(t, map[string]string{"LICENSE": "fixture", "PATENTS": "fixture", path: source})
	s, e := OpenSource(context.Background(), repo, pin)
	if e != nil {
		t.Fatal(e)
	}
	plan := Plan{InspectRoots: []string{"cmd/compile/internal/p"}, Units: []Unit{{Source: path, Functions: []string{"F"}, BuildVariant: "!unix"}}}
	r, e := Render(s, plan, t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	if strings.Contains(string(r.Files["internal/p/p_read.go"]), "go:build") {
		t.Fatal("constraint retained")
	}
	found := false
	for _, entry := range r.Ledger.Files {
		for _, d := range entry.Declarations {
			found = found || d == "selected-build-variant:!unix"
		}
	}
	if !found {
		t.Fatal("missing variant provenance")
	}
	for _, changed := range []string{strings.Replace(source, "!unix", "unix", 1), strings.Replace(source, "//go:build !unix\n", "", 1), strings.Replace(source, "\n\npackage", "\n// +build !unix\n\npackage", 1)} {
		s.cache[path] = []byte(changed)
		if _, e := Render(s, plan, t.TempDir()); e == nil {
			t.Fatal("changed constraint accepted")
		}
	}
}

func TestQualifiedNativeContextType(t *testing.T) {
	const q = "src/cmd/compile/internal/q/q.go"
	const p = "src/cmd/compile/internal/p/p.go"
	const source = `package p;import "cmd/compile/internal/q";var counter int;func Add(x *q.Context)int{counter+=x.Value;return counter}`
	repo, pin := fixtureRepo(t, map[string]string{"LICENSE": "fixture", "PATENTS": "fixture", q: `package q;type Context struct{Value int}`, p: source})
	s, e := OpenSource(context.Background(), repo, pin)
	if e != nil {
		t.Fatal(e)
	}
	plan := Plan{InspectRoots: []string{"cmd/compile/internal/p", "cmd/compile/internal/q"}, Units: []Unit{{Source: q, WholeFile: true}, {Source: p, WholeFile: true, Context: &ContextRule{Type: "Context", Constructor: "initialize", Initialize: true, Adapter: "adapter_context.go", Fields: map[string]string{"counter": "counter"}}}}}
	dir := t.TempDir()
	writeTestFile(t, dir, "internal/p/adapter_context.go", `package p;type Context struct{counter int}`)
	r, e := Render(s, plan, dir)
	if e != nil {
		t.Fatal(e)
	}
	if e = Install(dir, r); e != nil {
		t.Fatal(e)
	}
	writeTestFile(t, dir, "go.mod", "module go-backend-ssa\n\ngo 1.27.1\n")
	writeTestFile(t, dir, "internal/p/check_test.go", `package p;import("testing";"go-backend-ssa/internal/q");func TestContexts(t *testing.T){a,b:=&Context{},&Context{};x:=&q.Context{Value:3};if a.Add(x)!=3||a.Add(x)!=6||b.Add(x)!=3{t.Fatal("context binding")}}`)
	cmd := exec.Command("go", "test", "-race", "./...")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOWORK=off", "GOTOOLCHAIN=go1.27.1")
	if b, e := cmd.CombinedOutput(); e != nil {
		t.Fatalf("%v\n%s", e, b)
	}
	for _, changed := range []string{source + `;type Context struct{}`, strings.Replace(source, "counter+=x.Value", "Context:=x.Value;counter+=Context", 1), strings.Replace(source, "counter+=x.Value", "initialize:=x.Value;counter+=initialize", 1)} {
		s.cache[p] = []byte(changed)
		if _, e := Render(s, plan, dir); e == nil {
			t.Fatal("native local collision accepted")
		}
	}
}

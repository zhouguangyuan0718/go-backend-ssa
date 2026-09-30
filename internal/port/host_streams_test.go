package port

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestBoundCompilerSourceAndReportCalls(t *testing.T) {
	const source = `package p
import("os";"os/exec";"io")
func Input(path string)(io.ReadCloser,error){return os.Open(os.ExpandEnv(path))}
func Output(path string)(io.WriteCloser,error){return os.OpenFile(path,os.O_WRONLY|os.O_CREATE|os.O_TRUNC,0644)}
func Inputs()(string,string,string,string,error){a,b:=os.Getenv("GOSSAFUNC"),os.Getenv("GOSSADIR");wd,err:=os.Getwd();if err!=nil{return a,b,wd,"",err};dot,err:=exec.LookPath("dot");return a,b,wd,dot,err}
func Directory(path string)error{return os.MkdirAll(path,0755)}
`
	const adapter = `package p
import("io";"io/fs")
type Context struct {Read func(string)(io.ReadCloser,error);Write func(string,int,fs.FileMode)(io.WriteCloser,error);Env func(string)string;Expand func(string)string;WD func()(string,error);Find func(string)(string,error);Dir func(string,fs.FileMode)error}
func(c *Context)read(p string)(io.ReadCloser,error){return c.Read(p)}
func(c *Context)write(p string,f int,m fs.FileMode)(io.WriteCloser,error){return c.Write(p,f,m)}
func(c *Context)env(k string)string{return c.Env(k)}
func(c *Context)expand(p string)string{return c.Expand(p)}
func(c *Context)wd()(string,error){return c.WD()}
func(c *Context)find(p string)(string,error){return c.Find(p)}
func(c *Context)dir(p string,m fs.FileMode)error{return c.Dir(p,m)}
`
	const path = "src/cmd/compile/internal/p/p.go"
	repo, pin := fixtureRepo(t, map[string]string{"LICENSE": "fixture", "PATENTS": "fixture", path: source})
	s, err := OpenSource(context.Background(), repo, pin)
	if err != nil {
		t.Fatal(err)
	}
	plan := Plan{InspectRoots: []string{"cmd/compile/internal/p"}, Units: []Unit{{Source: path, WholeFile: true, Context: &ContextRule{Type: "Context", Constructor: "initialize", Initialize: true, Selective: true, Adapter: "adapter_context.go"}, HostCalls: []HostCallRule{
		{Package: "os", Function: "Open", Method: "read"}, {Package: "os", Function: "OpenFile", Method: "write"}, {Package: "os", Function: "ExpandEnv", Method: "expand"}, {Package: "os", Function: "Getwd", Method: "wd"}, {Package: "os", Function: "MkdirAll", Method: "dir"}, {Package: "os", Function: "Getenv", Method: "env", StringArguments: []string{"GOSSAFUNC", "GOSSADIR"}}, {Package: "os/exec", Function: "LookPath", Method: "find", StringArgument: "dot"},
	}}}}
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
	writeTestFile(t, dir, "internal/p/check_test.go", `package p
import("bytes";"errors";"fmt";"io";"io/fs";"os";"strings";"sync";"testing")
type stream struct{bytes.Buffer;closed int};func(s *stream)Close()error{s.closed++;return nil}
func TestStreams(t *testing.T){var wg sync.WaitGroup;for i:=range 8{wg.Go(func(){var input,output stream;fmt.Fprint(&input,i);var order []string;fail:=errors.New("injected");c:=&Context{Read:func(p string)(io.ReadCloser,error){order=append(order,"open");if p!=fmt.Sprint(i,"/source"){t.Error(p)};return &input,nil},Write:func(p string,f int,m fs.FileMode)(io.WriteCloser,error){if p!="report"||f!=os.O_WRONLY|os.O_CREATE|os.O_TRUNC||m!=0644{t.Error(p,f,m)};return &output,nil},Expand:func(p string)string{order=append(order,"expand");return strings.ReplaceAll(p,"$ROOT",fmt.Sprint(i))},Env:func(k string)string{return fmt.Sprint(i,k)},WD:func()(string,error){return "cwd",nil},Find:func(p string)(string,error){if p!="dot"{t.Error(p)};return "bin/dot",nil},Dir:func(p string,m fs.FileMode)error{if p!="dir"||m!=0755{t.Error(p,m)};return fail}};r,err:=c.Input("$ROOT/source");if err!=nil{t.Error(err)};b,_:=io.ReadAll(r);r.Close();w,err:=c.Output("report");if err!=nil{t.Error(err)};w.Write(b);w.Close();if fmt.Sprint(order)!="[expand open]"||input.closed!=1||output.closed!=1||output.String()!=fmt.Sprint(i){t.Error("stream lifecycle",order,input.closed,output.closed,output.String())};a,b2,wd,dot,err:=c.Inputs();if a!=fmt.Sprint(i,"GOSSAFUNC")||b2!=fmt.Sprint(i,"GOSSADIR")||wd!="cwd"||dot!="bin/dot"||err!=nil{t.Error(a,b2,wd,dot,err)};if c.Directory("dir")!=fail{t.Error("directory error")};c.Read=func(string)(io.ReadCloser,error){return nil,fail};if r,err:=c.Input("$ROOT/source");r!=nil||err!=fail{t.Error("read error")};c.Write=func(string,int,fs.FileMode)(io.WriteCloser,error){return nil,fail};if w,err:=c.Output("report");w!=nil||err!=fail{t.Error("write error")};c.WD=func()(string,error){return "",fail};c.Find=func(string)(string,error){t.Error("lookup after cwd error");return "",nil};if _,_,_,_,err:=c.Inputs();err!=fail{t.Error("cwd error")}})};wg.Wait()}
`)
	cmd := exec.Command("go", "test", "-race", "./...")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOWORK=off", "GOTOOLCHAIN=go1.27.1")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	for name, changed := range map[string]string{
		"unreviewed env":         strings.Replace(source, "GOSSAFUNC", "HOME", 1),
		"unused env key":         strings.Replace(source, "GOSSADIR", "GOSSAFUNC", 1),
		"dynamic env":            strings.Replace(source, `os.Getenv("GOSSADIR")`, `os.Getenv(a)`, 1),
		"lookup alias":           strings.Replace(source, `exec.LookPath("dot")`, `func()(string,error){f:=exec.LookPath;return f("dot")}()`, 1),
		"unreviewed executable":  strings.Replace(source, `exec.LookPath("dot")`, `exec.LookPath("sh")`, 1),
		"unreviewed open flag":   strings.Replace(source, "os.O_TRUNC", "os.O_APPEND", 1),
		"open flag outside call": source + `func Bad()int{return os.O_TRUNC}`,
		"native file method":     source + `func Bad(f *os.File)uintptr{return f.Fd()}`,
	} {
		t.Run(name, func(t *testing.T) {
			s.cache[path] = []byte(changed)
			defer func() { s.cache[path] = []byte(source) }()
			if _, err := Render(s, plan, dir); err == nil {
				t.Fatal("unreviewed source accepted")
			}
		})
	}
	for name, changed := range map[string]string{
		"reader close missing": strings.ReplaceAll(adapter, "io.ReadCloser", "io.Reader"),
		"writer close missing": strings.ReplaceAll(adapter, "io.WriteCloser", "io.Writer"),
		"flag type widened":    strings.ReplaceAll(strings.ReplaceAll(adapter, "func(string,int,fs.FileMode)", "func(string,int64,fs.FileMode)"), "f int,", "f int64,"),
	} {
		t.Run(name, func(t *testing.T) {
			writeTestFile(t, dir, "internal/p/adapter_context.go", changed)
			defer writeTestFile(t, dir, "internal/p/adapter_context.go", adapter)
			if _, err := Render(s, plan, dir); err == nil {
				t.Fatal("invalid adapter accepted")
			}
		})
	}
	for _, rule := range []HostCallRule{
		{Package: "os", Function: "Getenv", Method: "env", StringArguments: []string{"a", "a"}},
		{Package: "os", Function: "Getenv", Method: "env", StringArguments: []string{""}},
		{Package: "os", Function: "Getenv", Method: "env", StringArgument: "a", StringArguments: []string{"b"}},
		{Package: "os/exec", Function: "LookPath", Method: "find"},
		{Package: "os", Function: "Open", Method: "read", StringArguments: []string{"a"}},
	} {
		p := plan
		p.Units = append([]Unit(nil), plan.Units...)
		p.Units[0].HostCalls = []HostCallRule{rule}
		if p.Validate() == nil {
			t.Fatal("invalid host rule accepted", rule)
		}
	}
}

func TestBoundFileMetadata(t *testing.T) {
	const path = "src/cmd/compile/internal/p/p.go"
	const source = `package p
import "os"
func Size(path string)(int64,error){f,e:=os.Open(path);if e!=nil{return 0,e};defer f.Close();i,e:=f.Stat();if e!=nil{return 0,e};return i.Size(),nil}
`
	const adapter = `package p
import "io/fs"
type Context struct{Open func(string)(fs.File,error)}
func(c *Context)open(p string)(fs.File,error){return c.Open(p)}
`
	repo, pin := fixtureRepo(t, map[string]string{"LICENSE": "fixture", "PATENTS": "fixture", path: source})
	s, e := OpenSource(context.Background(), repo, pin)
	if e != nil {
		t.Fatal(e)
	}
	plan := Plan{InspectRoots: []string{"cmd/compile/internal/p"}, Units: []Unit{{Source: path, WholeFile: true, Context: &ContextRule{Type: "Context", Constructor: "initialize", Initialize: true, Selective: true, Adapter: "adapter_context.go"}, HostCalls: []HostCallRule{{Package: "os", Function: "Open", Method: "open"}}}}}
	dir := t.TempDir()
	writeTestFile(t, dir, "internal/p/adapter_context.go", adapter)
	r, e := Render(s, plan, dir)
	if e != nil {
		t.Fatal(e)
	}
	if e := Install(dir, r); e != nil {
		t.Fatal(e)
	}
	writeTestFile(t, dir, "go.mod", "module go-backend-ssa\n\ngo 1.27.1\n")
	writeTestFile(t, dir, "internal/p/check_test.go", `package p
import("fmt";"io/fs";"sync";"testing";"testing/fstest")
func TestMetadata(t *testing.T){var w sync.WaitGroup;for i:=range 8{w.Go(func(){data:=fmt.Sprint(i);c:=&Context{Open:func(p string)(fs.File,error){return (fstest.MapFS{"input":&fstest.MapFile{Data:[]byte(data)}}).Open(p)}};n,e:=c.Size("input");if e!=nil||n!=int64(len(data)){t.Error(n,e)}})};w.Wait()}
`)
	cmd := exec.Command("go", "test", "-race", "./...")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOWORK=off", "GOTOOLCHAIN=go1.27.1")
	if out, e := cmd.CombinedOutput(); e != nil {
		t.Fatalf("%v\n%s", e, out)
	}
	for name, changed := range map[string]string{"descriptor": strings.Replace(source, "return i.Size(),nil", "_ = f.Fd();return i.Size(),nil", 1), "filesystem": source + `func Bad(p string)error{return os.Remove(p)}`} {
		t.Run(name, func(t *testing.T) {
			s.cache[path] = []byte(changed)
			defer func() { s.cache[path] = []byte(source) }()
			if _, e := Render(s, plan, dir); e == nil {
				t.Fatal("unreviewed host method accepted")
			}
		})
	}
	for name, changed := range map[string]string{"native-file": strings.ReplaceAll(strings.ReplaceAll(adapter, `"io/fs"`, `"os"`), "fs.File", "*os.File"), "wrong-argument": strings.ReplaceAll(adapter, "p string", "p []byte")} {
		t.Run(name, func(t *testing.T) {
			writeTestFile(t, dir, "internal/p/adapter_context.go", changed)
			defer writeTestFile(t, dir, "internal/p/adapter_context.go", adapter)
			if _, e := Render(s, plan, dir); e == nil {
				t.Fatal("invalid adapter accepted")
			}
		})
	}
}

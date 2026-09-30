package port

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestOwnedAppendReportFlags(t *testing.T) {
	const path = "src/cmd/compile/internal/p/p.go"
	const source = `package p
import("os";"io")
func Output(fresh bool)(io.WriteCloser,error){mode:=os.O_APPEND|os.O_RDWR;if fresh{mode=os.O_CREATE|os.O_TRUNC|os.O_RDWR};return os.OpenFile("report",mode,0777)}
`
	repo, pin := fixtureRepo(t, map[string]string{"LICENSE": "fixture", "PATENTS": "fixture", path: source})
	s, e := OpenSource(context.Background(), repo, pin)
	if e != nil {
		t.Fatal(e)
	}
	u := Unit{Source: path, WholeFile: true, Context: &ContextRule{Type: "Context", Constructor: "initialize", Initialize: true, Selective: true, Adapter: "adapter_context.go"}, HostCalls: []HostCallRule{{Package: "os", Function: "OpenFile", Method: "write"}}}
	for _, binding := range [][2]string{{"O_APPEND", "Append"}, {"O_RDWR", "ReadWrite"}, {"O_CREATE", "Create"}, {"O_TRUNC", "Truncate"}} {
		u.HostConstants = append(u.HostConstants, HostConstantRule{Package: "os", Constant: binding[0], Field: binding[1]})
	}
	plan := Plan{InspectRoots: []string{"cmd/compile/internal/p"}, Units: []Unit{u}}
	dir := t.TempDir()
	writeTestFile(t, dir, "internal/p/adapter_context.go", `package p
import("io";"io/fs";"os")
type Context struct{Append,ReadWrite,Create,Truncate int;Write func(string,int,fs.FileMode)(io.WriteCloser,error)}
func Defaults()*Context{return &Context{Append:os.O_APPEND,ReadWrite:os.O_RDWR,Create:os.O_CREATE,Truncate:os.O_TRUNC}}
func(c *Context)write(p string,f int,m fs.FileMode)(io.WriteCloser,error){return c.Write(p,f,m)}
`)
	r, e := Render(s, plan, dir)
	if e != nil {
		t.Fatal(e)
	}
	if e = Install(dir, r); e != nil {
		t.Fatal(e)
	}
	writeTestFile(t, dir, "go.mod", "module go-backend-ssa\n\ngo 1.27.1\n")
	writeTestFile(t, dir, "internal/p/adapter_test.go", `package p
import("io";"io/fs";"os";"testing")
func TestFlags(t *testing.T){for _,fresh:=range []bool{false,true}{for _,offset:=range []int{0,1<<20}{c:=&Context{Append:os.O_APPEND|offset,ReadWrite:os.O_RDWR,Create:os.O_CREATE|offset,Truncate:os.O_TRUNC};c.Write=func(p string,f int,m fs.FileMode)(io.WriteCloser,error){want:=os.O_APPEND|os.O_RDWR|offset;if fresh{want=os.O_CREATE|os.O_TRUNC|os.O_RDWR|offset};if p!="report"||f!=want||m!=0777{t.Fatal(p,f,m)};return nil,nil};c.Output(fresh)}}}
`)
	cmd := exec.Command("go", "test", "./internal/p")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOWORK=off", "GOTOOLCHAIN=go1.27.1")
	if b, e := cmd.CombinedOutput(); e != nil {
		t.Fatalf("%v\n%s", e, b)
	}
	plan.Units[0].HostConstants = plan.Units[0].HostConstants[:3]
	if _, e := Render(s, plan, dir); e == nil || !strings.Contains(e.Error(), "unclassified environment") {
		t.Fatalf("unbound flag: %v", e)
	}
	for _, name := range []string{"O_EXCL", "O_SYNC", "PathSeparator"} {
		bad := u
		bad.HostConstants = []HostConstantRule{{Package: "os", Constant: name, Field: "Append"}}
		plan.Units = []Unit{bad}
		if e := plan.Validate(); e == nil {
			t.Fatalf("unreviewed host flag %s accepted", name)
		}
	}
}

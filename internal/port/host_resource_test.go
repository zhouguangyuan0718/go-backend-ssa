package port

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestBoundHostResource(t *testing.T) {
	source := `package p
 import("bytes";"io";"os/exec")
 func Run(input string)(string,error){
  cmd:=exec.Command("tool","--format")
  pipe,err:=cmd.StdinPipe();if err!=nil{return "",err}
  var output,errors bytes.Buffer;cmd.Stdout=&output;cmd.Stderr=&errors
  if err:=cmd.Start();err!=nil{return "",err};io.WriteString(pipe,input);pipe.Close()
  if err:=cmd.Wait();err!=nil{return errors.String(),err};return output.String(),nil
 }
 `
	repo, pin := fixtureRepo(t, map[string]string{"LICENSE": "fixture", "PATENTS": "fixture", "src/cmd/compile/internal/p/p.go": source})
	s, err := OpenSource(context.Background(), repo, pin)
	if err != nil {
		t.Fatal(err)
	}
	unit := Unit{Source: "src/cmd/compile/internal/p/p.go", WholeFile: true, Context: &ContextRule{Type: "Context", Constructor: "initialize", Adapter: "adapter_context.go", Selective: true}, HostResources: []HostResourceRule{{Package: "os/exec", Factory: "Command", Method: "command", Members: []string{"StdinPipe", "Start", "Wait", "Stdout", "Stderr"}}}}
	plan := Plan{InspectRoots: []string{"cmd/compile/internal/p"}, Units: []Unit{unit}}
	dir := t.TempDir()
	adapter := `package p
 import "os/exec"
 type Context struct{Factory func(string,...string)*exec.Cmd}
 func(c *Context)command(name string,args ...string)*exec.Cmd{return c.Factory(name,args...)}
 `
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
 import("fmt";"io";"os";"os/exec";"sync";"testing")
 func TestHelperProcess(t *testing.T){if os.Getenv("SYNCGO_RESOURCE_HELPER")==""{return};fmt.Print(os.Getenv("SYNCGO_RESOURCE_HELPER"));io.Copy(os.Stdout,os.Stdin);os.Exit(0)}
 func TestRun(t *testing.T){var wg sync.WaitGroup;for i:=0;i<8;i++{wg.Go(func(){prefix:=fmt.Sprint(i,":");c:=&Context{Factory:func(name string,args ...string)*exec.Cmd{if name!="tool"||len(args)!=1||args[0]!="--format"{t.Error("factory arguments")};cmd:=exec.Command(os.Args[0],"-test.run=^TestHelperProcess$");cmd.Env=append(os.Environ(),"SYNCGO_RESOURCE_HELPER="+prefix);return cmd}};got,err:=c.Run("input");if err!=nil||got!=prefix+"input"{t.Error(got,err)}})};wg.Wait()}
 `)
	cmd := exec.Command("go", "test", "-race", "./...")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOWORK=off", "GOTOOLCHAIN=go1.27.1")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	t.Run("renamed local and import", func(t *testing.T) {
		renamed := strings.Replace(source, `"os/exec"`, `process "os/exec"`, 1)
		renamed = strings.ReplaceAll(renamed, "exec.Command", "process.Command")
		renamed = strings.ReplaceAll(renamed, "cmd", "ownedProcess")
		s.cache[unit.Source] = []byte(renamed)
		defer func() { s.cache[unit.Source] = []byte(source) }()
		if _, err := Render(s, plan, dir); err != nil {
			t.Fatal(err)
		}
	})
	for name, change := range map[string]string{
		"reassignment":      strings.Replace(source, "pipe,err:=cmd.StdinPipe()", "cmd=exec.Command(\"second\");pipe,err:=cmd.StdinPipe()", 1),
		"alias":             strings.Replace(source, "pipe,err:=cmd.StdinPipe()", "other:=cmd;_ = other;pipe,err:=cmd.StdinPipe()", 1),
		"method value":      strings.Replace(source, "if err:=cmd.Start()", "start:=cmd.Start;if err:=start()", 1),
		"unlisted member":   strings.Replace(source, "if err:=cmd.Start()", "cmd.Dir=\"/\";if err:=cmd.Start()", 1),
		"resource literal":  source + `func Other(){_ = &exec.Cmd{}}`,
		"foreign resource":  source + `func Other(cmd *exec.Cmd){cmd.Start()}`,
		"returned resource": strings.Replace(source, "pipe,err:=cmd.StdinPipe()", "consume(cmd);pipe,err:=cmd.StdinPipe()", 1) + `func consume(cmd *exec.Cmd){}`,
	} {
		t.Run(name, func(t *testing.T) {
			s.cache[unit.Source] = []byte(change)
			if _, err := Render(s, plan, dir); err == nil {
				t.Fatal("unclassified host operation accepted")
			}
		})
	}
	s.cache[unit.Source] = []byte(source)
	t.Run("wrong adapter signature", func(t *testing.T) {
		writeTestFile(t, dir, "internal/p/adapter_context.go", strings.Replace(strings.Replace(adapter, "args ...string)*exec.Cmd{return", "args ...any)*exec.Cmd{return", 1), "return c.Factory(name,args...)", "return c.Factory(name)", 1))
		if _, err := Render(s, plan, dir); err == nil || !strings.Contains(err.Error(), "host resource adapter signature mismatch") {
			t.Fatalf("invalid adapter result: %v", err)
		}
	})
}

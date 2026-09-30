package port

import (
	"context"
	"os"
	"os/exec"
	"testing"
)

func TestBoundFlagLookup(t *testing.T) {
	const path = "src/cmd/compile/internal/p/p.go"
	source := `package p;import "flag";func Value(name string) string { f:=flag.Lookup(name);if f==nil{return ""};return f.Value.String() }`
	repo, pin := fixtureRepo(t, map[string]string{"LICENSE": "fixture", "PATENTS": "fixture", path: source})
	s, err := OpenSource(context.Background(), repo, pin)
	if err != nil {
		t.Fatal(err)
	}
	plan := Plan{InspectRoots: []string{"cmd/compile/internal/p"}, Units: []Unit{{Source: path, WholeFile: true, Context: &ContextRule{Type: "Context", Constructor: "initialize", Initialize: true, Selective: true, Adapter: "adapter_context.go"}, HostCalls: []HostCallRule{{Package: "flag", Function: "Lookup", Method: "lookupFlag"}}}}}
	dir := t.TempDir()
	writeTestFile(t, dir, "internal/p/adapter_context.go", `package p;import "flag";type Context struct{Flags interface{Lookup(string)*flag.Flag}};func(c *Context)lookupFlag(name string)*flag.Flag{return c.Flags.Lookup(name)}`)
	r, err := Render(s, plan, dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := Install(dir, r); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, dir, "go.mod", "module go-backend-ssa\n\ngo 1.27.1\n")
	writeTestFile(t, dir, "internal/p/check_test.go", `package p;import("flag";"testing");func TestFlags(t *testing.T){a,b:=&Context{flag.NewFlagSet("a",flag.ContinueOnError)},&Context{flag.NewFlagSet("b",flag.ContinueOnError)};a.Flags.(*flag.FlagSet).String("mode","left","");b.Flags.(*flag.FlagSet).String("mode","right","");if a.Value("mode")!="left"||b.Value("mode")!="right"||a.Value("missing")!=""{t.Fatal("flag sets crossed")}}`)
	cmd := exec.Command("go", "test", "-race", "./...")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOWORK=off", "GOTOOLCHAIN=go1.27.1")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}

	for name, extra := range map[string]string{
		"process parser":   `;func Bad(){flag.Parse()}`,
		"value mutation":   `;func Bad(){flag.Lookup("mode").Value.Set("changed")}`,
		"process flag set": `;func Bad()*flag.FlagSet{return flag.CommandLine}`,
	} {
		t.Run(name, func(t *testing.T) {
			s.cache[path] = []byte(source + extra)
			if _, err := Render(s, plan, dir); err == nil {
				t.Fatal("unreviewed flag operation accepted")
			}
		})
	}
}

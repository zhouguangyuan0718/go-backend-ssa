package port

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestExistingOwnerField(t *testing.T) {
	const code = `package p
 type Context struct{}
 func NewContext()*Context{return &Context{}}
 type Config struct{Context *Context}
 var count int
 func(c *Config)Check()int{count++;return count}
 func Configured()*Config{return new(Config)}
 func Use(c *Config)int{before:=count;return before+c.Check()}
 `
	repo, pin := fixtureRepo(t, map[string]string{"LICENSE": "fixture", "PATENTS": "fixture", "src/cmd/compile/internal/p/p.go": code})
	s, err := OpenSource(context.Background(), repo, pin)
	if err != nil {
		t.Fatal(err)
	}
	plan := Plan{InspectRoots: []string{"cmd/compile/internal/p"}, Units: []Unit{{Source: "src/cmd/compile/internal/p/p.go", WholeFile: true, Owner: &OwnerRule{Existing: true, Types: []string{"Config"}, Context: "Context", Field: "Context"}, Context: &ContextRule{Existing: true, Selective: true, Type: "Context", Constructor: "NewContext", Fields: map[string]string{"count": "count"}}}}}
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
 import "testing"
 func TestNativeField(t *testing.T){a,b:=NewContext(),NewContext();conf:=a.Configured();if conf.Context!=a||conf.Check()!=1||b.Configured().Check()!=1||a.Use(conf)!=3{t.Fatal("existing context field")};defer func(){if recover()==nil{t.Fatal("foreign config accepted")}}();b.Use(conf)}
 `)
	cmd := exec.Command("go", "test", "-race", "./...")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOWORK=off", "GOTOOLCHAIN=go1.27.1")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	for _, tc := range []struct{ from, to, want string }{{"Context *Context", "other *Context", "not found"}, {"Context *Context", "Context int", "incompatible type"}} {
		s.cache[plan.Units[0].Source] = []byte(strings.Replace(code, tc.from, tc.to, 1))
		if _, err := Render(s, plan, t.TempDir()); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Fatalf("got %v, want %s", err, tc.want)
		}
	}
}

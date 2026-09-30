package port

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestDeferredUnselectedInit(t *testing.T) {
	const source = `package p
var selected = 7
var unrelated = []int{3,4}
func init(){ validate() }
func validate(){ if unrelated[0]!=3 {panic("bad table")} }
func Read() int {return selected}
`
	const path = "src/cmd/compile/internal/p/p.go"
	repo, pin := fixtureRepo(t, map[string]string{"LICENSE": "fixture", "PATENTS": "fixture", path: source})
	src, err := OpenSource(context.Background(), repo, pin)
	if err != nil {
		t.Fatal(err)
	}
	unit := Unit{Source: path, Functions: []string{"Read"}, Context: &ContextRule{Type: "Context", Constructor: "newContext", Selective: true, Adapter: "adapter_context.go", Fields: map[string]string{"selected": "selected"}, DeferInit: true}}
	plan := Plan{InspectRoots: []string{"cmd/compile/internal/p"}, Units: []Unit{unit}}
	dir := t.TempDir()
	writeTestFile(t, dir, "internal/p/adapter_context.go", "package p;type Context struct{selected int}")
	r, err := Render(src, plan, dir)
	if err != nil {
		t.Fatal(err)
	}
	code := string(r.Files["internal/p/p.go"])
	if strings.Contains(code, "validate") || strings.Contains(code, "unrelated") {
		t.Fatal("unselected init retained")
	}
	if err := Install(dir, r); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, dir, "go.mod", "module go-backend-ssa\n\ngo 1.27.1\n")
	writeTestFile(t, dir, "internal/p/adapter_test.go", `package p;import "testing";func TestContexts(t *testing.T){a,b:=newContext(),newContext();if a.Read()!=7||b.Read()!=7{t.Fatal("lost initializer")};a.selected=9;if b.Read()!=7{t.Fatal("shared state")}}`)
	cmd := exec.Command("go", "test", "./...")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOWORK=off", "GOTOOLCHAIN=go1.27.1")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	for _, tc := range []struct{ name, body string }{
		{"direct state", "selected++"},
		{"direct function", "_ = Read()"},
		{"helper state", "validate()"},
		{"initializer chain", "_ = indirect"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data := strings.Replace(source, "validate() }", tc.body+" }", 1)
			if tc.name == "helper state" {
				data = strings.Replace(data, "if unrelated[0]!=3", "if selected!=7", 1)
			}
			if tc.name == "initializer chain" {
				data += "\nvar indirect=selected\n"
			}
			src.cache[path] = []byte(data)
			defer func() { src.cache[path] = []byte(source) }()
			if _, err := Render(src, plan, dir); err == nil || !strings.Contains(err.Error(), "deferred init references selected") {
				t.Fatalf("expected dependency rejection, got %v", err)
			}
		})
	}
	t.Run("default rejects", func(t *testing.T) {
		u := unit
		c := *unit.Context
		c.DeferInit = false
		u.Context = &c
		p := plan
		p.Units = []Unit{u}
		if _, err := Render(src, p, dir); err == nil || !strings.Contains(err.Error(), "init requires") {
			t.Fatalf("%v", err)
		}
	})
	for _, whole := range []bool{false, true} {
		u := unit
		c := *unit.Context
		u.Context = &c
		if whole {
			u.WholeFile = true
			u.Functions = nil
		} else {
			c.Init = true
		}
		p := plan
		p.Units = []Unit{u}
		if err := p.Validate(); err == nil {
			t.Fatal("invalid deferred init accepted")
		}
	}
}

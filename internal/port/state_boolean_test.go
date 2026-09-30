package port

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestExternalComparisonBooleanState(t *testing.T) {
	const host = `package host;var Enabled = (readTarget() == "amd64") || !(readTarget() != "arm64")`
	const source = `package p;import "cmd/compile/internal/host";func Enabled()bool{return host.Enabled}`
	const hostPath = "src/cmd/compile/internal/host/host.go"
	repo, pin := fixtureRepo(t, map[string]string{"LICENSE": "fixture", "PATENTS": "fixture", hostPath: host, "src/cmd/compile/internal/p/p.go": source})
	s, err := OpenSource(context.Background(), repo, pin)
	if err != nil {
		t.Fatal(err)
	}
	plan := Plan{InspectRoots: []string{"cmd/compile/internal/p"}, Units: []Unit{{Source: "src/cmd/compile/internal/p/p.go", WholeFile: true, Context: &ContextRule{Type: "Context", Constructor: "newContext", Adapter: "adapter_context.go", Selective: true}, State: []StateRule{{Source: hostPath, Variable: "Enabled", Field: "enabled"}}}}}
	dir := t.TempDir()
	writeTestFile(t, dir, "internal/p/adapter_context.go", `package p;type Context struct{enabled bool}`)
	r, err := Render(s, plan, dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, data := range r.Files {
		if strings.Contains(string(data), "readTarget") {
			t.Fatal("host initializer escaped into output")
		}
	}
	if err := Install(dir, r); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, dir, "go.mod", "module go-backend-ssa\n\ngo 1.27.1\n")
	writeTestFile(t, dir, "internal/p/adapter_test.go", `package p;import "testing";func TestState(t *testing.T){a,b:=newContext(),newContext();a.enabled=true;if !a.Enabled()||b.Enabled(){t.Fatal("host state not bound")};b.enabled=true;a.enabled=false;if a.Enabled()||!b.Enabled(){t.Fatal("state leaked")}}`)
	cmd := exec.Command("go", "test", "./...")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOWORK=off", "GOTOOLCHAIN=go1.27.1")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	for _, expr := range []string{"named", "!named", "named || (readTarget() == \"amd64\")", "1 + 2"} {
		s.cache[hostPath] = []byte("package host;var Enabled = " + expr)
		if _, err := Render(s, plan, dir); err == nil {
			t.Fatalf("inferred unknown type for %s", expr)
		}
	}
	s.cache[hostPath] = []byte(host)
	writeTestFile(t, dir, "internal/p/adapter_context.go", `package p;type Context struct{enabled int}`)
	if _, err := Render(s, plan, dir); err == nil {
		t.Fatal("boolean adapter type drift accepted")
	}
}

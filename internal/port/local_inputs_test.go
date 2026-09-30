package port

import (
	"context"
	"go/ast"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestLocalHostInputs(t *testing.T) {
	const path = "src/cmd/compile/internal/p/p.go"
	const source = `package p;import "os";type feature struct{Level int};func(f *feature)Touch(){f.Level++};var Target=os.Getenv("SYNCGO_MUST_NOT_READ");var Feature=hostFeature();func hostFeature()feature{panic("must not execute")};func Label()string{return Target};func Level()int{return Feature.Level}`
	repo, pin := fixtureRepo(t, map[string]string{"LICENSE": "fixture", "PATENTS": "fixture", path: source})
	s, err := OpenSource(context.Background(), repo, pin)
	if err != nil {
		t.Fatal(err)
	}
	u := Unit{Source: path, Functions: []string{"Label", "Level"}, Declarations: []string{"feature", "feature.Touch"}, Context: &ContextRule{Type: "Context", Constructor: "newContext", Adapter: "adapter_context.go", Selective: true}, State: []StateRule{{Source: path, Variable: "Target", Field: "Target", Input: true}, {Source: path, Variable: "Feature", Field: "Feature", Input: true}}}
	plan := Plan{InspectRoots: []string{"cmd/compile/internal/p"}, Units: []Unit{u}}
	dir := t.TempDir()
	writeTestFile(t, dir, "internal/p/adapter_context.go", `package p;type Context struct{Target string;Feature feature}`)
	r, err := Render(s, plan, dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := Install(dir, r); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, dir, "go.mod", "module go-backend-ssa\n\ngo 1.27.1\n")
	writeTestFile(t, dir, "internal/p/check_test.go", `package p;import "testing";func TestInputs(t *testing.T){for i:=range 8{t.Run(string(rune('a'+i)),func(t *testing.T){t.Parallel();a,b:=&Context{Target:"left",Feature:feature{3}},&Context{Target:"right",Feature:feature{9}};if a.Label()!="left"||b.Label()!="right"||a.Level()!=3||b.Level()!=9{t.Fatal("inputs crossed")}})}}`)
	cmd := exec.Command("go", "test", "-race", "./...")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOWORK=off", "GOTOOLCHAIN=go1.27.1")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	for name, body := range map[string]string{"assign": `func Bad(){Target="host"}`, "increment": `func Bad(){Feature.Level++}`, "address": `func Bad(){_ = &Feature}`, "pointer method": `func Bad(){Feature.Touch()}`} {
		t.Run(name, func(t *testing.T) {
			s.cache[path] = []byte(source + ";" + body)
			p := plan
			p.Units = append([]Unit(nil), plan.Units...)
			p.Units[0].Functions = append(append([]string(nil), u.Functions...), "Bad")
			if _, err := Render(s, p, dir); err == nil {
				t.Fatal("mutable input accepted")
			}
		})
	}
	s.cache[path] = []byte(strings.Replace(source, "var Feature=hostFeature()", "var Feature *feature", 1))
	if _, err := Render(s, plan, dir); err == nil {
		t.Fatal("reference-valued input accepted")
	}
	s.cache[path] = []byte(source)
	writeTestFile(t, dir, "internal/p/adapter_context.go", `package p;type Context struct{Target int;Feature feature}`)
	if _, err := Render(s, plan, dir); err == nil {
		t.Fatal("wrong input field type accepted")
	}
}
func TestBootstrapConstantTypeProof(t *testing.T) {
	const path = "src/cmd/tool/generate.go"
	source := "package main;import(\"fmt\";\"io\");func generate(w io.Writer){fmt.Fprintf(w,\"const version = `%s`\\n\",\"arbitrary version\")}"
	repo, pin := fixtureRepo(t, map[string]string{"LICENSE": "fixture", "PATENTS": "fixture", path: source})
	s, err := OpenSource(context.Background(), repo, pin)
	if err != nil {
		t.Fatal(err)
	}
	spec := &ast.ValueSpec{Names: []*ast.Ident{ast.NewIdent("Version")}, Values: []ast.Expr{ast.NewIdent("version")}}
	typ, digest, err := generatedConstantType(s, spec, path)
	if err != nil || typ.(*ast.Ident).Name != "string" || digest != hash([]byte(source)) {
		t.Fatal(typ, digest, err)
	}
	for _, changed := range []string{strings.Replace(source, "const version", "const other", 1), strings.Replace(source, "`%s`", "%d", 1), strings.Replace(source, "fmt.Fprintf(w,", "fmt:=struct{Fprintf func(io.Writer,string,...any)}{};fmt.Fprintf(w,", 1)} {
		s.cache[path] = []byte(changed)
		if _, _, err := generatedConstantType(s, spec, path); err == nil {
			t.Fatal("unproven bootstrap constant accepted")
		}
	}
}

func TestBootstrapConstantBindingProvenance(t *testing.T) {
	const generator = "src/cmd/tool/generate.go"
	const host = "src/internal/host/config.go"
	const use = "src/cmd/compile/internal/p/p.go"
	proof := "package main;import(\"fmt\";\"io\");func generate(w io.Writer){fmt.Fprintf(w,\"const version = `%s`\\n\",\"pinned\")}"
	repo, pin := fixtureRepo(t, map[string]string{"LICENSE": "fixture", "PATENTS": "fixture", generator: proof, host: `package host;var Version=version`, use: `package p;import "internal/host";func Read()string{return host.Version}`})
	s, err := OpenSource(context.Background(), repo, pin)
	if err != nil {
		t.Fatal(err)
	}
	p := Plan{InspectRoots: []string{"cmd/compile/internal/p"}, Units: []Unit{{Source: use, Functions: []string{"Read"}, Context: &ContextRule{Type: "Context", Constructor: "newContext", Selective: true, Adapter: "adapter_context.go"}, State: []StateRule{{Source: host, Variable: "Version", Field: "Version", TypeSource: generator}}}}}
	dir := t.TempDir()
	writeTestFile(t, dir, "internal/p/adapter_context.go", `package p;type Context struct{Version string}`)
	r, err := Render(s, p, dir)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, file := range r.Ledger.Files {
		if file.Source == use {
			found = true
			if file.SignatureSources[generator] != hash([]byte(proof)) {
				t.Fatal("bootstrap proof missing from provenance")
			}
		}
	}
	if !found {
		t.Fatal("source missing")
	}
	s.cache[generator] = []byte(strings.Replace(proof, "const version", "const renamed", 1))
	if _, err := Render(s, p, dir); err == nil {
		t.Fatal("changed bootstrap signature accepted")
	}
}

func TestGeneratedLocalConstantInput(t *testing.T) {
	const generator = "src/cmd/tool/generate.go"
	const source = "src/cmd/compile/internal/p/main.go"
	const code = `package p;func Read()string{return DefaultTarget}`
	const proof = "package main;import(\"fmt\";\"io\");func generate(w io.Writer){fmt.Fprintf(w,\"const DefaultTarget = `%s`\\n\",\"host choice\")}"
	repo, pin := fixtureRepo(t, map[string]string{"LICENSE": "fixture", "PATENTS": "fixture", generator: proof, source: code})
	input, err := OpenSource(context.Background(), repo, pin)
	if err != nil {
		t.Fatal(err)
	}
	p := Plan{InspectRoots: []string{"cmd/compile/internal/p"}, Units: []Unit{{Source: source, Functions: []string{"Read"}, Context: &ContextRule{Type: "Context", Constructor: "newContext", Selective: true, Adapter: "adapter_context.go"}, State: []StateRule{{Source: source, Variable: "DefaultTarget", Field: "DefaultTarget", Input: true, TypeSource: generator}}}}}
	dir := t.TempDir()
	writeTestFile(t, dir, "internal/p/adapter_context.go", `package p;type Context struct{DefaultTarget string}`)
	r, err := Render(input, p, dir)
	if err != nil {
		t.Fatal(err)
	}
	data := string(r.Files["internal/p/main.go"])
	if !strings.Contains(data, "syncgoCtx.DefaultTarget") || strings.Contains(data, "host choice") {
		t.Fatal(data)
	}
	seen := false
	for _, file := range r.Ledger.Files {
		if file.Source == source {
			seen = file.SignatureSources[generator] == hash([]byte(proof))
		}
	}
	if !seen {
		t.Fatal("missing generated input proof")
	}
	for _, changed := range []string{strings.Replace(proof, "const DefaultTarget", "const Other", 1), strings.Replace(proof, "`%s`", "%d", 1)} {
		input.cache[generator] = []byte(changed)
		if _, err := Render(input, p, dir); err == nil {
			t.Fatal("unproven generated input accepted")
		}
	}
	input.cache[generator] = []byte(proof)
	input.cache[source] = []byte(`package p;func Read()string{DefaultTarget="changed";return DefaultTarget}`)
	if _, err := Render(input, p, dir); err == nil {
		t.Fatal("generated constant mutation accepted")
	}
}

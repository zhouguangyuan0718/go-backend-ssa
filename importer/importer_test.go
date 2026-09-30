package importer_test

import (
	"bytes"
	"go-backend-ssa/importer"
	"go-backend-ssa/syntax"
	"go-backend-ssa/types2"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestPublicExportImporter(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "dep.go")
	archive := filepath.Join(dir, "dep.a")
	if e := os.WriteFile(file, []byte(`package dep;type Box[T any]struct{V T};func Identity[T any](x T)T{return x}`), 0600); e != nil {
		t.Fatal(e)
	}
	cmd := exec.Command("go", "tool", "compile", "-pack", "-p", "example/dep", "-o", archive, file)
	cmd.Env = append(os.Environ(), "GOWORK=off", "GOTOOLCHAIN=go1.27.1")
	if b, e := cmd.CombinedOutput(); e != nil {
		t.Fatalf("%v\n%s", e, b)
	}
	b, e := os.ReadFile(archive)
	if e != nil {
		t.Fatal(e)
	}
	start, end := bytes.Index(b, []byte("\n$$B\nu")), bytes.Index(b, []byte("\n$$\n"))
	if start < 0 || end <= start {
		t.Fatal("invalid compiler export")
	}
	checker := types2.NewContext()
	deps := importer.NewContext(checker)
	imported := deps.ReadExport("example/dep", string(b[start+len("\n$$B\nu"):end]))
	if imported.Context() != checker {
		t.Fatal("package ownership")
	}
	f, e := syntax.Parse(syntax.NewFileBase("use.go"), strings.NewReader(`package use;import "example/dep";var X=dep.Identity(dep.Box[int]{V:42})`), nil, nil, syntax.CheckBranches)
	if e != nil {
		t.Fatal(e)
	}
	cfg := types2.Config{Context: checker, GoVersion: "go1.27", Importer: deps, Sizes: checker.SizesFor("gc", "amd64")}
	pkg, e := cfg.Check("example/use", []*syntax.File{f}, nil)
	if e != nil {
		t.Fatal(e)
	}
	if got := pkg.Scope().Lookup("X").Type().String(); got != "example/dep.Box[int]" {
		t.Fatal(got)
	}
}

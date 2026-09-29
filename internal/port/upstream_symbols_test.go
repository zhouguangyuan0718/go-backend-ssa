package port

import (
	"bytes"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// Keep upstream's symbol ordering assertions, giving its package construction
// an explicit per-test Universe in place of the original global package map.
func testUpstreamSymbols(t *testing.T, s *Source, r *Rendered, root string) {
	t.Helper()
	data, err := s.Read("src/cmd/compile/internal/types/sym_test.go")
	if err != nil {
		t.Fatal(err)
	}
	fs := token.NewFileSet()
	f, err := parser.ParseFile(fs, "sym_test.go", data, parser.ParseComments)
	if err != nil {
		t.Fatal(err)
	}
	for _, im := range f.Imports {
		if im.Path.Value == `"cmd/compile/internal/types"` {
			im.Path.Value = `"go-backend-ssa/internal/types"`
		}
	}
	for _, d := range f.Decls {
		fn, ok := d.(*ast.FuncDecl)
		if !ok {
			continue
		}
		used := false
		ast.Inspect(fn, func(n ast.Node) bool {
			if sel, ok := n.(*ast.SelectorExpr); ok {
				if id, ok := sel.X.(*ast.Ident); ok && id.Name == "types" && sel.Sel.Name == "NewPkg" {
					sel.X = ast.NewIdent("universe")
					used = true
				}
			}
			return true
		})
		if used {
			expr, err := parser.ParseExpr("types.NewUniverse(8, 8, 1<<50)")
			if err != nil {
				t.Fatal(err)
			}
			stmt := &ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent("universe")}, Tok: token.DEFINE, Rhs: []ast.Expr{expr}}
			fn.Body.List = append([]ast.Stmt{stmt}, fn.Body.List...)
		}
	}
	var out bytes.Buffer
	if err := format.Node(&out, fs, f); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	for _, name := range Migration().SupportFiles {
		data, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			t.Fatal(err)
		}
		writeTestFile(t, dir, name, string(data))
	}
	for name, data := range r.Files {
		writeTestFile(t, dir, name, string(data))
	}
	for _, u := range Migration().Units {
		if u.Context != nil {
			data, err := os.ReadFile(filepath.Join(root, u.adapterPath()))
			if err != nil {
				t.Fatal(err)
			}
			writeTestFile(t, dir, u.adapterPath(), string(data))
		}
	}
	writeTestFile(t, dir, "go.mod", "module go-backend-ssa\n\ngo 1.27.1\n")
	writeTestFile(t, dir, "internal/types/adapter_upstream_sym_test.go", out.String())
	cmd := exec.Command("go", "test", "-race", "-v", "./internal/types")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOWORK=off", "GOTOOLCHAIN=go1.27.1")
	b, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("upstream symbol suite (%s): %v\n%s", s.Pin.Tag, err, b)
	}
	t.Logf("upstream symbol suite at %s:\n%s", s.Pin.Tag, b)
}

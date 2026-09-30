package port

import (
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"strings"
	"testing"
)

func TestSelectedPackageReportsIndependentTypeErrors(t *testing.T) {
	fs := token.NewFileSet()
	file, err := parser.ParseFile(fs, "missing.go", `package p
func A(){firstMissing()}
func B(){secondMissing()}
`, 0)
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = checkTypes(fs, "p", []*ast.File{file}, importer.Default())
	if err == nil || !strings.Contains(err.Error(), "missing.go:2:") || !strings.Contains(err.Error(), "firstMissing") || !strings.Contains(err.Error(), "missing.go:3:") || !strings.Contains(err.Error(), "secondMissing") {
		t.Fatal("incomplete selected-package diagnostics", err)
	}
}

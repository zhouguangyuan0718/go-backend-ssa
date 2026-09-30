package gcintegration

import (
	"bytes"
	"strings"
	"testing"
)

func TestLibraryImportAliases(t *testing.T) {
	source := []byte("// go-backend-ssa/compiler stays a comment\npackage p\nimport(\n\"fmt\"\nnamed \"go-backend-ssa/compiler\"\n. `go-backend-ssa/types`\n_ \"other/go-backend-ssa/a\"\n)\nconst message=\"go-backend-ssa/compiler\"\n")
	result, err := rewriteLibraryImports("p.go", source)
	if err != nil {
		t.Fatal(err)
	}
	expected := strings.ReplaceAll(string(source), "named \"go-backend-ssa/compiler\"", "named \"go-backend-ssa.invalid/compiler\"")
	expected = strings.ReplaceAll(expected, ". `go-backend-ssa/types`", ". \"go-backend-ssa.invalid/types\"")
	if string(result) != expected {
		t.Fatalf("non-import text changed:\n%s", result)
	}
	clear(result)
	if !bytes.Contains(source, []byte("named \"go-backend-ssa/compiler\"")) {
		t.Fatal("source storage modified")
	}
	if _, err := rewriteLibraryImports("bad.go", []byte("package p;import(")); err == nil {
		t.Fatal("invalid imports accepted")
	}
}

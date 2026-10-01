package compiler_test

import (
	"bytes"
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// Audit the actual public compiler dependency closure, including facades and
// handwritten composition outside the generated-package audit. Standard-library
// implementation state is outside the compiler's ownership contract.
func TestCompilerProductionClosure(t *testing.T) {
	cmd := exec.Command("go", "list", "-deps", "-json", ".")
	data, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	packages, files := 0, 0
	for {
		var pkg struct {
			ImportPath, Dir           string
			GoFiles, CgoFiles, SFiles []string
		}
		err := dec.Decode(&pkg)
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if !strings.HasPrefix(pkg.ImportPath, "go-backend-ssa/") {
			continue
		}
		packages++
		if len(pkg.CgoFiles) > 0 || len(pkg.SFiles) > 0 {
			t.Errorf("unreviewed native implementation in %s", pkg.ImportPath)
		}
		for _, name := range pkg.GoFiles {
			files++
			path := filepath.Join(pkg.Dir, name)
			fset := token.NewFileSet()
			f, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
			if err != nil {
				t.Fatal(err)
			}
			report := func(pos token.Pos, format string, args ...any) {
				t.Helper()
				t.Errorf("%s: "+format, append([]any{fset.Position(pos)}, args...)...)
			}
			imports := map[string]string{}
			for _, i := range f.Imports {
				path, err := strconv.Unquote(i.Path.Value)
				if err != nil {
					t.Fatal(err)
				}
				if strings.HasPrefix(path, "cmd/") {
					report(i.Pos(), "host compiler dependency %s", path)
				}
				name := filepath.Base(path)
				if i.Name != nil {
					name = i.Name.Name
				}
				if name == "." && path != "go-backend-ssa/internal/go/types/errors" {
					report(i.Pos(), "dot import obscures host dependencies")
				}
				imports[name] = path
			}
			for _, decl := range f.Decls {
				switch d := decl.(type) {
				case *ast.GenDecl:
					if d.Tok == token.VAR {
						report(d.Pos(), "package variable can retain compilation state")
					}
				case *ast.FuncDecl:
					if d.Recv == nil && d.Name.Name == "init" {
						report(d.Pos(), "package init can install hidden state")
					}
				}
			}
			for _, group := range f.Comments {
				for _, comment := range group.List {
					if strings.HasPrefix(comment.Text, "//go:linkname") {
						report(comment.Pos(), "unreviewed linkname")
					}
				}
			}
			ast.Inspect(f, func(n ast.Node) bool {
				sel, ok := n.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				id, ok := sel.X.(*ast.Ident)
				if !ok || id.Obj != nil {
					return true
				}
				path := imports[id.Name]
				name := sel.Sel.Name
				forbidden := false
				switch path {
				case "os":
					// File modes are constants; PathError is a passive diagnostic type.
					forbidden = !(strings.HasPrefix(name, "O_") || name == "PathError" || name == "FileMode")
				case "os/exec":
					forbidden = name != "Cmd" // Commands must come from a supplied provider.
				case "flag":
					forbidden = !(name == "NewFlagSet" || name == "ContinueOnError" || name == "Flag" || name == "Value" || name == "Getter")
				case "log", "syscall":
					forbidden = true
				case "runtime":
					forbidden = !(name == "GOOS" || name == "GOARCH" || name == "MemStats" || name == "Error" || name == "Caller" || name == "Callers" || name == "CallersFrames" || name == "Frames" || name == "Frame" || name == "FuncForPC" || name == "Stack")
				}
				if forbidden {
					report(sel.Pos(), "implicit host dependency %s.%s", path, name)
				}
				return true
			})
		}
	}
	if packages < 10 || files < 100 {
		t.Fatal("compiler closure unexpectedly incomplete", packages, files)
	}
	t.Logf("audited %d module packages and %d production files", packages, files)
}

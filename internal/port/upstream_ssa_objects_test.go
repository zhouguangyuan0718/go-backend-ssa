package port

import (
	"context"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// Build the comparison from actual pinned opcode/register initializers. New
// upstream assembler references must compile against the migrated packages too.
func ssaObjectsSnapshot(s *Source) ([]byte, error) {
	data, err := s.Read("src/cmd/compile/internal/ssa/opGen.go")
	if err != nil {
		return nil, err
	}
	f, err := parser.ParseFile(token.NewFileSet(), "opGen.go", data, 0)
	if err != nil {
		return nil, err
	}
	imports := map[string]string{}
	for _, i := range f.Imports {
		p, err := strconv.Unquote(i.Path.Value)
		if err != nil {
			return nil, err
		}
		if p != "cmd/internal/obj" && !strings.HasPrefix(p, "cmd/internal/obj/") {
			return nil, fmt.Errorf("unexpected opcode import %q", p)
		}
		name := path.Base(p)
		if i.Name != nil {
			name = i.Name.Name
		}
		imports[name] = p
	}
	used := map[string]bool{}
	refs := map[string]bool{}
	for _, d := range f.Decls {
		g, ok := d.(*ast.GenDecl)
		if !ok || (g.Tok != token.CONST && g.Tok != token.VAR) {
			continue
		}
		for _, sp := range g.Specs {
			v := sp.(*ast.ValueSpec)
			for _, expr := range v.Values {
				ast.Inspect(expr, func(n ast.Node) bool {
					sel, ok := n.(*ast.SelectorExpr)
					if !ok {
						return true
					}
					id, ok := sel.X.(*ast.Ident)
					if ok && imports[id.Name] != "" {
						used[id.Name] = true
						refs[id.Name+"."+sel.Sel.Name] = true
					}
					return true
				})
			}
		}
	}
	if len(refs) == 0 {
		return nil, fmt.Errorf("no assembler constants referenced by opGen")
	}
	var aliases, names []string
	for a := range used {
		aliases = append(aliases, a)
	}
	sort.Strings(aliases)
	for n := range refs {
		names = append(names, n)
	}
	sort.Strings(names)
	var b strings.Builder
	b.WriteString("package ssa\nimport(\"encoding/json\";\"fmt\";\"testing\";")
	for _, a := range aliases {
		fmt.Fprintf(&b, "%s %q;", a, strings.Replace(imports[a], "cmd/internal/obj", "SSA_OBJ_ROOT", 1))
	}
	b.WriteString(")\nfunc TestSyncgoNativeSnapshot(t *testing.T){out:=[]string{\n")
	for _, n := range names {
		fmt.Fprintf(&b, "fmt.Sprintf(%q,int64(%s)),\n", n+":%d", n)
	}
	b.WriteString("};data,err:=json.Marshal(out);if err!=nil{t.Fatal(err)};fmt.Println(\"SYNCGO_IR=\"+string(data))}\n")
	return []byte(b.String()), nil
}

func TestPinnedSSAObjects(t *testing.T) {
	repo := os.Getenv("GO_SSA_SOURCE_REPO")
	if repo == "" {
		t.Skip("set GO_SSA_SOURCE_REPO for pinned SSA assembler constants")
	}
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	for _, tag := range []string{"go1.27.1", "go1.27.0"} {
		t.Run(tag, func(t *testing.T) {
			pin := Baseline()
			if tag != pin.Tag {
				pin.Tag = tag
				commit, err := git(context.Background(), repo, nil, "rev-parse", "refs/tags/"+tag+"^{commit}")
				if err != nil {
					t.Fatal(err)
				}
				pin.Commit = strings.TrimSpace(string(commit))
			}
			s, err := OpenSource(context.Background(), repo, pin)
			if err != nil {
				t.Fatal(err)
			}
			r, err := Render(s, Migration(), root)
			if err != nil {
				t.Fatal(err)
			}
			testUpstreamNativeGraph(t, s, r, root, "ssa", "ssa_objects")
		})
	}
}

package port

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strconv"
	"testing"
)

// Compare entire type bodies, including private fields and callback signatures,
// directly with pinned source. Only import qualifiers may differ. This catches
// a tempting partial Link/Prog replacement before native-backend integration.
func testUpstreamObjectDeclarations(t *testing.T, s *Source, r *Rendered) {
	t.Helper()
	units := objectDeclarations()
	units = append(units, Unit{Source: "src/cmd/internal/obj/link.go", Declarations: []string{"Link", "LinkArch", "Prog", "LSym", "Addr"}})
	imports := Migration().importMap(s.Pin.TargetModule)
	for _, u := range units {
		data, err := s.Read(u.Source)
		if err != nil {
			t.Fatal(err)
		}
		parse := func(data []byte, original bool) map[string]string {
			fs := token.NewFileSet()
			f, err := parser.ParseFile(fs, u.Source, data, 0)
			if err != nil {
				t.Fatal(err)
			}
			names := map[string]string{}
			for _, im := range f.Imports {
				p, _ := strconv.Unquote(im.Path.Value)
				name := p
				for i := len(name) - 1; i >= 0; i-- {
					if name[i] == '/' {
						name = name[i+1:]
						break
					}
				}
				if im.Name != nil {
					name = im.Name.Name
				}
				if original && imports[p] != "" {
					p = imports[p]
				}
				names[name] = p
			}
			out := map[string]string{}
			for _, d := range f.Decls {
				g, ok := d.(*ast.GenDecl)
				if !ok || g.Tok != token.TYPE {
					continue
				}
				for _, spec := range g.Specs {
					ts := spec.(*ast.TypeSpec)
					ast.Inspect(ts.Type, func(n ast.Node) bool {
						if sel, ok := n.(*ast.SelectorExpr); ok {
							if id, ok := sel.X.(*ast.Ident); ok && names[id.Name] != "" {
								id.Name = strconv.Quote(names[id.Name])
							}
						}
						return true
					})
					out[ts.Name.Name] = nodeText(fs, ts.Type)
				}
			}
			return out
		}
		original, ported := parse(data, true), parse(r.Files[u.output()], false)
		for _, name := range u.Declarations {
			want, ok := original[name]
			if !ok {
				continue
			}
			if got := ported[name]; got != want {
				t.Fatalf("native declaration changed: %s %s\nported: %s\noriginal: %s", u.Source, name, got, want)
			}
		}
	}
}

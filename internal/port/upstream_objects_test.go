package port

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strconv"
	"strings"
	"testing"
)

// Compare entire type bodies, including private fields and callback signatures,
// directly with pinned source. Only import qualifiers may differ. This catches
// a tempting partial Link/Prog replacement before native-backend integration.
func testUpstreamObjectDeclarations(t *testing.T, s *Source, r *Rendered) {
	t.Helper()
	testUpstreamObjectMethods(t, s, r)
	units := objectDeclarations()
	units = append(units, runtimeABIMigration()...)
	units = append(units, Unit{Source: "src/cmd/compile/internal/noder/reader.go", Declarations: []string{"readerDict", "readerMethodExprInfo", "pkgReaderIndex"}})
	units = append(units, Unit{Source: "src/cmd/compile/internal/reflectdata/reflect.go", Declarations: []string{"ptabEntry", "typeSig", "typeAndStr"}})
	units = append(units, Unit{Source: "src/cmd/compile/internal/reflectdata/alg.go", Declarations: []string{"eqSigBuilder"}})
	units = append(units, Unit{Source: "src/cmd/compile/internal/bitvec/bv.go", Declarations: []string{"BitVec", "Bulk"}})
	units = append(units, Unit{Source: "src/cmd/compile/internal/ir/node.go", Declarations: []string{"Op", "PragmaFlag"}})
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
					if !original && u.Source == "src/cmd/internal/obj/link.go" && ts.Name.Name == "Link" {
						st := ts.Type.(*ast.StructType)
						extra := st.Fields.List[len(st.Fields.List)-2:]
						embed, ok := extra[0].Type.(*ast.Ident)
						ptr, pok := extra[1].Type.(*ast.StarExpr)
						if !ok || embed.Name != "dataState" || len(extra[0].Names) != 0 || !pok || len(extra[1].Names) != 1 || extra[1].Names[0].Name != "bisectFIPS" {
							t.Fatal("unexpected Link state extension")
						}
						sel, sok := ptr.X.(*ast.SelectorExpr)
						if !sok || sel.Sel.Name != "Matcher" || names[sel.X.(*ast.Ident).Name] != "go-backend-ssa/internal/go/bisect" {
							t.Fatal("unexpected Link matcher type")
						}
						// Compare every original field and callback unchanged;
						// only these two reviewed instance fields are appended.
						st.Fields.List = st.Fields.List[:len(st.Fields.List)-2]
					}
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

// These native methods need only import relocation, not context rewriting.
// Compare their full signatures and bodies with immutable upstream source.
func testUpstreamObjectMethods(t *testing.T, s *Source, r *Rendered) {
	t.Helper()
	selected := map[string][]string{
		"src/cmd/compile/internal/bitvec/bv.go":    {"New", "NewBulk", "Bulk.Next", "BitVec.Copy", "BitVec.Next", "BitVec.IsEmpty", "BitVec.Count", "BitVec.Not", "BitVec.Or", "BitVec.And", "BitVec.AndNot", "BitVec.String", "BitVec.Clear"},
		"src/cmd/compile/internal/base/timings.go": {"Timings.append", "Timings.Start", "Timings.Stop", "Timings.AddEvent", "Timings.Write", "commonPrefix", "lines.add", "lines.write", "isnumber"},
	}
	for _, u := range Migration().Units {
		if u.sourcePackage() != "cmd/internal/obj" {
			continue
		}
		for _, name := range u.Declarations {
			if strings.Contains(name, ".") {
				selected[u.Source] = append(selected[u.Source], name)
			}
		}
		selected[u.Source] = append(selected[u.Source], u.Functions...)
	}
	for source, names := range selected {
		raw, err := s.Read(source)
		if err != nil {
			t.Fatal(err)
		}
		parse := func(data []byte) map[string]string {
			fs := token.NewFileSet()
			f, err := parser.ParseFile(fs, source, data, 0)
			if err != nil {
				t.Fatal(err)
			}
			out := map[string]string{}
			for _, d := range f.Decls {
				if fn, ok := d.(*ast.FuncDecl); ok {
					key := fn.Name.Name
					if fn.Recv != nil {
						key = receiverType(fn) + "." + key
					}
					out[key] = nodeText(fs, fn)
				}
			}
			return out
		}
		original, ported := parse(raw), parse(r.Files[portPath(source)])
		for name, body := range original {
			original[name] = strings.ReplaceAll(body, "src.NoXPos", "(src.XPos{})")
		}
		for _, name := range names {
			if original[name] == "" || ported[name] != original[name] {
				t.Fatalf("native object method changed: %s %s\noriginal: %s\nported: %s", source, name, original[name], ported[name])
			}
		}
	}
}

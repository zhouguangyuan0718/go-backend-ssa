package port

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strconv"
	"strings"
	"testing"
)

func TestPinnedObjectDeclarations(t *testing.T) {
	_, source, rendered := checkedInPinnedSnapshot(t)
	testUpstreamObjectDeclarations(t, source, rendered)
}

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
	units = append(units, Unit{Source: "src/cmd/internal/obj/link.go", Declarations: []string{"Link", "LinkArch", "Prog", "LSym", "Addr", "FuncInfo", "Pcln", "InlMark", "RegSpill", "JumpTable", "WasmImport", "WasmExport", "WasmFuncType", "WasmField", "WasmFieldType"}})
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
						// These reviewed fields replace native package state. Every
						// original Link field and callback is compared below.
						extra := []struct{ name, typ string }{
							{"", "dataState"}, {"aSpace", "[]opSet"}, {"bisectFIPS", "matcher"},
							{"inlHashLen", "int"}, {"opSuffixSpace", "[]opSuffixSet"},
							{"regListSpace", "[]regListSet"}, {"regSpace", "[]regSet"}, {"spcSpace", "[]spcSet"},
							{"textAttrStrings", "[19]struct { bit Attribute s string }"},
						}
						start := len(st.Fields.List) - len(extra)
						if start < 0 {
							t.Fatal("missing Link state extensions")
						}
						for i, want := range extra {
							field := st.Fields.List[start+i]
							name := ""
							if len(field.Names) == 1 {
								name = field.Names[0].Name
							}
							if len(field.Names) > 1 || name != want.name || field.Tag != nil {
								t.Fatal("unexpected Link state extension", want.name)
							}
							if want.typ == "matcher" {
								ptr, ok := field.Type.(*ast.StarExpr)
								if !ok {
									t.Fatal("invalid matcher pointer")
								}
								sel, ok := ptr.X.(*ast.SelectorExpr)
								if !ok {
									t.Fatal("invalid matcher selector")
								}
								id, ok := sel.X.(*ast.Ident)
								if !ok || sel.Sel.Name != "Matcher" || names[id.Name] != "go-backend-ssa/internal/go/bisect" {
									t.Fatal("unexpected Link matcher type")
								}
							} else if got := strings.Join(strings.Fields(nodeText(fs, field.Type)), " "); got != want.typ {
								t.Fatal("unexpected Link extension type", want.name, got)
							}
						}
						st.Fields.List = st.Fields.List[:start]
					}
					if !original && (u.Source == "src/cmd/internal/objabi/flag.go" && ts.Name.Name == "DebugFlag" || u.Source == "src/cmd/internal/sys/arch.go" && ts.Name.Name == "Arch") {
						st := ts.Type.(*ast.StructType)
						last := st.Fields.List[len(st.Fields.List)-1]
						if len(last.Names) != 1 || last.Names[0].Name != "compilation" || nodeText(fs, last.Type) != "*Context" || last.Tag != nil {
							t.Fatal("unexpected owner extension")
						}
						st.Fields.List = st.Fields.List[:len(st.Fields.List)-1]
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
				t.Errorf("native declaration changed: %s %s\nported: %s\noriginal: %s", u.Source, name, got, want)
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
			original[name] = strings.NewReplacer("src.NoXPos", "(src.XPos{})", "src.NoPos", "(src.Pos{})").Replace(body)
		}
		// Attribute.String now owns its constant display table through the
		// reviewed context resolver. Compare the complete native algorithm
		// after checking and removing only that explicit state binding.
		if source == "src/cmd/internal/obj/link.go" {
			const binding = "\tsyncgoCtx := attributeLink(a)\n\tif syncgoCtx == nil {\n\t\tpanic(\"uninitialized compilation\")\n\t}\n"
			body := ported["Attribute.String"]
			if strings.Count(body, binding) != 1 {
				t.Fatal("unexpected Attribute.String context binding")
			}
			body = strings.Replace(body, binding, "", 1)
			ported["Attribute.String"] = strings.ReplaceAll(body, "syncgoCtx.textAttrStrings", "textAttrStrings")
		}
		if source == "src/cmd/internal/obj/sym.go" {
			const guard = "\tif ctxt == nil || false {\n\t\tpanic(\"receiver has no compilation owner\")\n\t}\n\tctxt.checkNumberSyms()\n"
			body := ported["Link.NumberSyms"]
			if strings.Count(body, guard) != 1 || strings.Count(body, "ctxt.ObjectBuiltins.BuiltinIdx") != 1 {
				t.Fatal("unexpected NumberSyms instance bindings")
			}
			body = strings.Replace(body, guard, "", 1)
			ported["Link.NumberSyms"] = strings.Replace(body, "ctxt.ObjectBuiltins.BuiltinIdx", "goobj.BuiltinIdx", 1)
		}
		for _, name := range names {
			if original[name] == "" || ported[name] != original[name] {
				t.Errorf("native object method changed: %s %s\noriginal: %s\nported: %s", source, name, original[name], ported[name])
			}
		}
	}
}

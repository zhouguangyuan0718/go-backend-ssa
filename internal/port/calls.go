package port

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"path"
	"strings"
)

// Only upstream signatures are needed to type-check host references whose
// implementation or state storage is intentionally replaced. Bodies are omitted, as for external
// Go declarations; these packages exist only inside the generator's type checker.
// The real adapter's full signature is checked again after transformation.
func loadCallSignatures(s *Source, units []Unit, importer *packageImporter) (map[string]string, error) {
	selected := map[string]map[string]bool{}
	variables := map[string]map[string]bool{}
	for _, u := range units {
		for _, state := range u.State {
			if variables[state.Source] == nil {
				variables[state.Source] = map[string]bool{}
			}
			if selected[state.Source] == nil {
				selected[state.Source] = map[string]bool{}
			}
			variables[state.Source][state.Variable] = true
		}
		for _, call := range u.Calls {
			if selected[call.Source] == nil {
				selected[call.Source] = map[string]bool{}
			}
			selected[call.Source][call.Function] = true
		}
	}
	fs := token.NewFileSet()
	packages := map[string][]*ast.File{}
	hashes := map[string]string{}
	for _, source := range sortedKeys(selected) {
		data, err := s.Read(source)
		if err != nil {
			return nil, err
		}
		hashes[source] = hash(data)
		f, err := parser.ParseFile(fs, source, data, parser.ParseComments|parser.SkipObjectResolution)
		if err != nil {
			return nil, err
		}
		found := map[string]bool{}
		typeDecls := map[string]*ast.TypeSpec{}
		var decls []ast.Decl
		for _, declaration := range f.Decls {
			switch d := declaration.(type) {
			case *ast.FuncDecl:
				if d.Recv == nil && selected[source][d.Name.Name] {
					d.Body = nil
					decls = append(decls, d)
					found[d.Name.Name] = true
				}
			case *ast.GenDecl:
				if d.Tok == token.TYPE {
					for _, spec := range d.Specs {
						t := spec.(*ast.TypeSpec)
						typeDecls[t.Name.Name] = t
					}
					continue
				}
				if d.Tok == token.IMPORT {
					decls = append(decls, d)
					continue
				}
				if d.Tok != token.VAR {
					continue
				}
				var specs []ast.Spec
				for _, spec := range d.Specs {
					v := spec.(*ast.ValueSpec)
					for _, name := range v.Names {
						if variables[source][name.Name] {
							if len(v.Names) != 1 || v.Type == nil {
								return nil, fmt.Errorf("external state requires an explicitly typed single declaration: %s", name.Name)
							}
							v.Values = nil
							specs = append(specs, v)
							found[name.Name] = true
						}
					}
				}
				if len(specs) > 0 {
					d.Specs = specs
					decls = append(decls, d)
				}
			}
		}
		for name := range selected[source] {
			if !found[name] {
				return nil, fmt.Errorf("external function not found: %s", name)
			}
		}
		for name := range variables[source] {
			if !found[name] {
				return nil, fmt.Errorf("external state not found: %s", name)
			}
		}
		// Include only local type declarations reachable from the signatures.
		// Unrelated host structs can depend on compiler/process infrastructure.
		needed := map[string]bool{}
		var visit func(ast.Node)
		visit = func(node ast.Node) {
			ast.Inspect(node, func(n ast.Node) bool {
				if id, ok := n.(*ast.Ident); ok {
					if spec := typeDecls[id.Name]; spec != nil && !needed[id.Name] {
						needed[id.Name] = true
						visit(spec.Type)
					}
				}
				return true
			})
		}
		for _, decl := range decls {
			visit(decl)
		}
		for _, name := range sortedKeys(needed) {
			decls = append(decls, &ast.GenDecl{Tok: token.TYPE, Specs: []ast.Spec{typeDecls[name]}})
		}
		f.Decls = decls
		pruneImports(f)
		pkg := strings.TrimPrefix(path.Dir(source), "src/")
		packages[pkg] = append(packages[pkg], f)
	}
	for _, pkg := range sortedKeys(packages) {
		checked, _, err := checkTypes(fs, pkg, packages[pkg], importer)
		if err != nil {
			return nil, fmt.Errorf("upstream adapter signatures: %w", err)
		}
		// Preserve previously checked declarations (for example base.EnableTrace).
		if previous := importer.packages[pkg]; previous != nil {
			for _, name := range previous.Scope().Names() {
				if checked.Scope().Lookup(name) == nil {
					checked.Scope().Insert(previous.Scope().Lookup(name))
				}
			}
		}
		importer.packages[pkg] = checked
	}
	return hashes, nil
}

func checkCallAdapters(units []Unit, before *packageImporter, out *types.Package, imports map[string]string) error {
	for _, u := range units {
		for _, call := range u.Calls {
			sourcePkg := strings.TrimPrefix(path.Dir(call.Source), "src/")
			original := before.packages[sourcePkg].Scope().Lookup(call.Function)
			ctx := out.Scope().Lookup(u.Context.Type)
			if ctx == nil {
				return fmt.Errorf("missing adapter context")
			}
			adapter, _, _ := types.LookupFieldOrMethod(types.NewPointer(ctx.Type()), true, out, call.Method)
			if _, ok := adapter.(*types.Func); !ok {
				return fmt.Errorf("missing adapter method %s", call.Method)
			}
			want := types.TypeString(unnamedSignature(original.Type()), func(p *types.Package) string {
				if mapped := imports[p.Path()]; mapped != "" {
					return mapped
				}
				return p.Path()
			})
			if got := types.TypeString(unnamedSignature(adapter.Type()), nil); got != want {
				return fmt.Errorf("adapter signature mismatch for %s: got %s, want %s", call.Method, got, want)
			}
		}
	}
	return nil
}

func checkStateAdapters(units []Unit, before *packageImporter, out *types.Package, imports map[string]string) error {
	for _, u := range units {
		for _, state := range u.State {
			sourcePkg := strings.TrimPrefix(path.Dir(state.Source), "src/")
			original := before.packages[sourcePkg].Scope().Lookup(state.Variable)
			ctx := out.Scope().Lookup(u.Context.Type)
			if ctx == nil {
				return fmt.Errorf("missing state context")
			}
			adapter, _, _ := types.LookupFieldOrMethod(types.NewPointer(ctx.Type()), true, out, state.Field)
			field, ok := adapter.(*types.Var)
			if !ok || !field.IsField() {
				return fmt.Errorf("missing state field %s", state.Field)
			}
			want := types.TypeString(unnamedSignature(original.Type()), func(p *types.Package) string {
				if mapped := imports[p.Path()]; mapped != "" {
					return mapped
				}
				return p.Path()
			})
			if got := types.TypeString(field.Type(), nil); got != want {
				return fmt.Errorf("external state type mismatch for %s: got %s, want %s", state.Field, got, want)
			}
		}
	}
	return nil
}

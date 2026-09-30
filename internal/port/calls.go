package port

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"path"
	"strconv"
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
			variables[state.Source][stateRoot(state.Variable)] = true
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
		if strings.TrimPrefix(path.Dir(source), "src/") == units[0].sourcePackage() {
			if len(variables[source]) != 0 {
				return nil, fmt.Errorf("local state must use context fields")
			}
			continue // Checked with selected declarations by localCallSignatures.
		}
		f, err := parser.ParseFile(fs, source, data, parser.ParseComments|parser.SkipObjectResolution)
		if err != nil {
			return nil, err
		}
		if err := projectSignatureStructs(f, importer.projections[source]); err != nil {
			return nil, err
		}
		// A fully migrated dependency has already been checked from these exact
		// source declarations. Reuse those objects instead of recreating named
		// types in a signature-only package (which would split type identity).
		pkgPath := strings.TrimPrefix(path.Dir(source), "src/")
		if previous := importer.packages[pkgPath]; previous != nil {
			reuse := true
			for name := range selected[source] {
				obj := previous.Scope().Lookup(name)
				_, ok := obj.(*types.Func)
				reuse = reuse && ok && importer.sources[obj] == source
			}
			for name := range variables[source] {
				obj := previous.Scope().Lookup(name)
				ok := externalStateObject(obj)
				reuse = reuse && ok && importer.sources[obj] == source
			}
			if reuse {
				continue
			}
		}
		found := map[string]bool{}
		reused := map[string]bool{}
		if previous := importer.packages[pkgPath]; previous != nil {
			for name := range selected[source] {
				if obj, ok := previous.Scope().Lookup(name).(*types.Func); ok && importer.sources[obj] == source {
					reused[name] = true
					found[name] = true
				}
			}
			for name := range variables[source] {
				if obj := previous.Scope().Lookup(name); externalStateObject(obj) && importer.sources[obj] == source {
					reused[name] = true
					found[name] = true
				}
			}
		}
		typeDecls := map[string]*ast.TypeSpec{}
		var decls []ast.Decl
		for _, declaration := range f.Decls {
			switch d := declaration.(type) {
			case *ast.FuncDecl:
				if d.Recv == nil && selected[source][d.Name.Name] && !reused[d.Name.Name] {
					d.Body = nil
					decls = append(decls, d)
					found[d.Name.Name] = true
				}
			case *ast.GenDecl:
				if d.Tok == token.TYPE {
					for _, spec := range d.Specs {
						t := spec.(*ast.TypeSpec)
						if prev := importer.packages[pkgPath]; prev == nil || prev.Scope().Lookup(t.Name.Name) == nil {
							typeDecls[t.Name.Name] = t
						}
					}
					continue
				}
				if d.Tok == token.IMPORT {
					decls = append(decls, d)
					continue
				}
				if d.Tok != token.VAR && d.Tok != token.CONST {
					continue
				}
				var specs []ast.Spec
				for _, spec := range d.Specs {
					v := spec.(*ast.ValueSpec)
					for _, name := range v.Names {
						if variables[source][name.Name] && !reused[name.Name] {
							if len(v.Names) != 1 {
								return nil, fmt.Errorf("external state requires a single declaration: %s", name.Name)
							}
							if d.Tok == token.CONST {
								// Feature build files declare literal bool constants. Preserve
								// the original constant object for typed reference binding.
								if len(v.Values) != 1 {
									return nil, fmt.Errorf("external feature requires a literal boolean: %s", name.Name)
								}
								value, ok := v.Values[0].(*ast.Ident)
								if !ok || (value.Name != "true" && value.Name != "false") {
									return nil, fmt.Errorf("external feature requires a literal boolean: %s", name.Name)
								}
								if v.Type != nil {
									typ, ok := v.Type.(*ast.Ident)
									if !ok || typ.Name != "bool" {
										return nil, fmt.Errorf("external feature requires a boolean type: %s", name.Name)
									}
								}
								specs = append(specs, v)
								found[name.Name] = true
								continue
							}
							if v.Type == nil {
								typ, err := externalValueType(f, v, importer)
								if err != nil {
									return nil, fmt.Errorf("external state %s: %w", name.Name, err)
								}
								v.Type = typ
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
		checked := types.NewPackage(pkg, packages[pkg][0].Name.Name)
		if previous := importer.packages[pkg]; previous != nil {
			for _, name := range previous.Scope().Names() {
				checked.Scope().Insert(previous.Scope().Lookup(name))
			}
		}
		info := &types.Info{Defs: map[*ast.Ident]types.Object{}}
		config := types.Config{Importer: importer, GoVersion: "go1.27"}
		if err := types.NewChecker(&config, fs, checked, info).Files(packages[pkg]); err != nil {
			return nil, fmt.Errorf("upstream adapter signatures: %w", err)
		}
		for _, obj := range info.Defs {
			if obj != nil && obj.Parent() == checked.Scope() {
				importer.sources[obj] = fs.Position(obj.Pos()).Filename
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
			adapter, _, _ := lookupAdapterMember(types.NewPointer(ctx.Type()), out, call.Method)
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
			original := externalStateMember(before.packages[sourcePkg], state.Variable)
			if original == nil {
				return fmt.Errorf("missing original state %s", state.Variable)
			}
			ctx := out.Scope().Lookup(u.Context.Type)
			if ctx == nil {
				return fmt.Errorf("missing state context")
			}
			adapter, _, _ := lookupAdapterMember(types.NewPointer(ctx.Type()), out, state.Field)
			field, ok := adapter.(*types.Var)
			if !ok || !field.IsField() {
				return fmt.Errorf("missing state field %s", state.Field)
			}
			originalType := original.Type()
			if _, ok := original.(*types.Const); ok {
				originalType = types.Default(originalType)
			}
			want := types.TypeString(unnamedSignature(originalType), func(p *types.Package) string {
				if mapped := imports[p.Path()]; mapped != "" {
					return mapped
				}
				return p.Path()
			})
			if got := types.TypeString(unnamedSignature(field.Type()), nil); got != want {
				return fmt.Errorf("external state type mismatch for %s: got %s, want %s", state.Field, got, want)
			}
		}
	}
	return nil
}

// Infer a host variable's type from a direct call's declared single result.
// Its environment-reading initializer and arguments never enter the output.
func externalValueType(file *ast.File, spec *ast.ValueSpec, importer *packageImporter) (ast.Expr, error) {
	if len(spec.Values) != 1 {
		return nil, fmt.Errorf("requires an explicit type or single call initializer")
	}
	call, ok := spec.Values[0].(*ast.CallExpr)
	if !ok {
		return nil, fmt.Errorf("requires an explicit type or direct call initializer")
	}
	if id, ok := call.Fun.(*ast.Ident); ok {
		for _, d := range file.Decls {
			if fn, ok := d.(*ast.FuncDecl); ok && fn.Recv == nil && fn.Name.Name == id.Name {
				if fn.Type.TypeParams != nil || fn.Type.Results == nil || len(fn.Type.Results.List) != 1 || len(fn.Type.Results.List[0].Names) > 1 {
					return nil, fmt.Errorf("initializer requires a non-generic single-result function")
				}
				return fn.Type.Results.List[0].Type, nil
			}
		}
	}
	if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
		if id, ok := sel.X.(*ast.Ident); ok {
			for _, im := range file.Imports {
				p, _ := strconv.Unquote(im.Path.Value)
				name := path.Base(p)
				if im.Name != nil {
					name = im.Name.Name
				}
				if name != id.Name {
					continue
				}
				pkg, err := importer.Import(p)
				if err != nil {
					return nil, err
				}
				obj, ok := pkg.Scope().Lookup(sel.Sel.Name).(*types.Func)
				if !ok {
					return nil, fmt.Errorf("initializer is not a package function")
				}
				sig := obj.Type().(*types.Signature)
				if sig.TypeParams().Len() != 0 || sig.Results().Len() != 1 {
					return nil, fmt.Errorf("initializer requires a non-generic single-result function")
				}
				// Only basic results are needed for environment-derived host settings.
				typ, ok := sig.Results().At(0).Type().(*types.Basic)
				if !ok {
					return nil, fmt.Errorf("imported initializer requires a basic result")
				}
				return ast.NewIdent(typ.Name()), nil
			}
		}
	}
	return nil, fmt.Errorf("initializer function signature unavailable")
}

// Resolve an explicitly configured path through native per-instance inputs.
// Each component is checked by go/types; no textual symbol substitution occurs.
func lookupAdapterMember(typ types.Type, pkg *types.Package, path string) (types.Object, []int, bool) {
	var obj types.Object
	var indices []int
	var indirect bool
	for _, name := range strings.Split(path, ".") {
		obj, indices, indirect = types.LookupFieldOrMethod(typ, true, pkg, name)
		if obj == nil {
			return nil, nil, false
		}
		typ = obj.Type()
	}
	return obj, indices, indirect
}

func localCallSignatures(s *Source, units []Unit, fs *token.FileSet, original []*ast.File) ([]*ast.File, error) {
	selected := map[string]map[string]bool{}
	present := map[string]string{}
	for _, f := range original {
		for _, d := range f.Decls {
			if fn, ok := d.(*ast.FuncDecl); ok && fn.Recv == nil {
				present[fn.Name.Name] = fs.PositionFor(fn.Pos(), false).Filename
			}
		}
	}
	for _, u := range units {
		for _, call := range u.Calls {
			if strings.TrimPrefix(path.Dir(call.Source), "src/") != u.sourcePackage() {
				continue
			}
			if source := present[call.Function]; source != "" {
				if source != call.Source {
					return nil, fmt.Errorf("local binding source mismatch for %s: %s != %s", call.Function, source, call.Source)
				}
			} else {
				if selected[call.Source] == nil {
					selected[call.Source] = map[string]bool{}
				}
				selected[call.Source][call.Function] = true
			}
		}
	}
	var files []*ast.File
	for _, source := range sortedKeys(selected) {
		data, err := s.Read(source)
		if err != nil {
			return nil, err
		}
		f, err := selectFile(fs, source, data, Unit{Source: source, Functions: sortedKeys(selected[source])})
		if err != nil {
			return nil, err
		}
		for _, decl := range f.Decls {
			if fn, ok := decl.(*ast.FuncDecl); ok {
				fn.Body = nil
			}
		}
		pruneImports(f)
		files = append(files, f)
	}
	return files, nil
}

// Features retain upstream constant identity while their uses read an instance bool.
func externalStateObject(obj types.Object) bool {
	switch obj := obj.(type) {
	case *types.Var:
		return true
	case *types.Const:
		typ, ok := obj.Type().Underlying().(*types.Basic)
		return ok && typ.Info()&types.IsBoolean != 0
	}
	return false
}

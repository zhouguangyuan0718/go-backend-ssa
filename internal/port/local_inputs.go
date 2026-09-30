package port

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
)

// Local host inputs use the same native signature extraction as external
// settings, but their declarations exist only in the generator type checker.
// Their host initializers never run or enter generated production code.
func localInputSignatures(s *Source, units []Unit, fs *token.FileSet, importer *packageImporter) ([]*ast.File, error) {
	requested := map[string]map[string]bool{}
	for _, u := range units {
		for _, state := range u.State {
			if state.Input {
				if requested[state.Source] == nil {
					requested[state.Source] = map[string]bool{}
				}
				requested[state.Source][state.Variable] = true
			}
		}
	}
	var out []*ast.File
	for _, source := range sortedKeys(requested) {
		data, err := s.Read(source)
		if err != nil {
			return nil, err
		}
		f, err := parser.ParseFile(fs, source, data, parser.SkipObjectResolution)
		if err != nil {
			return nil, err
		}
		var decls []ast.Decl
		found := map[string]bool{}
		for _, decl := range f.Decls {
			if gen, ok := decl.(*ast.GenDecl); ok && gen.Tok == token.VAR {
				for _, spec := range gen.Specs {
					v := spec.(*ast.ValueSpec)
					for _, name := range v.Names {
						if requested[source][name.Name] {
							if len(v.Names) != 1 {
								return nil, fmt.Errorf("host input requires a single declaration: %s", name.Name)
							}
							if v.Type == nil {
								v.Type, err = externalValueType(f, v, importer)
								if err != nil {
									return nil, fmt.Errorf("host input %s: %w", name.Name, err)
								}
							}
							v.Values = nil
							decls = append(decls, &ast.GenDecl{Tok: token.VAR, Specs: []ast.Spec{v}})
							found[name.Name] = true
						}
					}
				}
			}
		}
		for _, name := range sortedKeys(requested[source]) {
			if found[name] {
				continue
			}
			// Some native inputs are constants emitted by cmd/dist rather than
			// declarations in the tracked file. Prove the scalar type from that
			// pinned generator before providing a type-check-only input variable.
			proof := ""
			for _, u := range units {
				for _, state := range u.State {
					if state.Input && state.Source == source && state.Variable == name {
						if proof != "" && proof != state.TypeSource {
							return nil, fmt.Errorf("conflicting host input proof: %s", name)
						}
						proof = state.TypeSource
					}
				}
			}
			if proof == "" {
				return nil, fmt.Errorf("host input not found: %s", name)
			}
			id := &ast.Ident{NamePos: f.Name.Pos(), Name: name}
			v := &ast.ValueSpec{Names: []*ast.Ident{id}, Values: []ast.Expr{ast.NewIdent(name)}}
			v.Type, _, err = generatedConstantType(s, v, proof)
			if err != nil {
				return nil, err
			}
			v.Values = nil
			decls = append(decls, &ast.GenDecl{Tok: token.VAR, Specs: []ast.Spec{v}})
		}
		f.Decls = decls
		pruneImports(f)
		out = append(out, f)
	}
	return out, nil
}
func checkLocalInputs(files []*ast.File, info *types.Info, pkg *types.Package, units []Unit) error {
	inputs := map[types.Object]bool{}
	var valueType func(types.Type) bool
	valueType = func(t types.Type) bool {
		switch t := t.Underlying().(type) {
		case *types.Basic:
			return t.Kind() != types.UnsafePointer
		case *types.Array:
			return valueType(t.Elem())
		case *types.Struct:
			for i := 0; i < t.NumFields(); i++ {
				if !valueType(t.Field(i).Type()) {
					return false
				}
			}
			return true
		}
		return false
	}
	for _, u := range units {
		for _, state := range u.State {
			if state.Input {
				obj := pkg.Scope().Lookup(state.Variable)
				if _, ok := obj.(*types.Var); !ok || !valueType(obj.Type()) {
					return fmt.Errorf("host input must have a value-only type: %s", state.Variable)
				}
				inputs[obj] = true
			}
		}
	}
	if len(inputs) == 0 {
		return nil
	}
	var root func(ast.Expr) types.Object
	root = func(e ast.Expr) types.Object {
		switch e := e.(type) {
		case *ast.Ident:
			return info.Uses[e]
		case *ast.SelectorExpr:
			return root(e.X)
		case *ast.IndexExpr:
			return root(e.X)
		case *ast.ParenExpr:
			return root(e.X)
		}
		return nil
	}
	var failure error
	for _, file := range files {
		ast.Inspect(file, func(n ast.Node) bool {
			if failure != nil {
				return false
			}
			reject := func(e ast.Expr) {
				if obj := root(e); inputs[obj] {
					failure = fmt.Errorf("host input cannot be mutated or addressed: %s", obj.Name())
				}
			}
			switch n := n.(type) {
			case *ast.AssignStmt:
				for _, lhs := range n.Lhs {
					reject(lhs)
				}
			case *ast.IncDecStmt:
				reject(n.X)
			case *ast.UnaryExpr:
				if n.Op == token.AND {
					reject(n.X)
				}
			case *ast.SelectorExpr:
				if sel := info.Selections[n]; sel != nil && sel.Kind() == types.MethodVal {
					if fn, ok := sel.Obj().(*types.Func); ok {
						if _, pointer := fn.Type().(*types.Signature).Recv().Type().(*types.Pointer); pointer {
							reject(n.X)
						}
					}
				}
			}
			return failure == nil
		})
	}
	return failure
}

package port

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"path"
	"strings"
)

func stateRoot(name string) string { root, _, _ := strings.Cut(name, "."); return root }

// A projection is deliberately limited to a directly declared field of a named
// struct. It exposes no partial native value to production code.
func collectStateProjections(units []Unit) (map[string]map[string]map[string]bool, error) {
	out := map[string]map[string]map[string]bool{}
	whole := map[string]bool{}
	for _, u := range units {
		for _, r := range u.State {
			root, field, projected := strings.Cut(r.Variable, ".")
			key := r.Source + ":" + root
			if !projected {
				whole[key] = true
				continue
			}
			if strings.Contains(field, ".") {
				return nil, fmt.Errorf("state projection requires a direct field: %s", r.Variable)
			}
			if out[r.Source] == nil {
				out[r.Source] = map[string]map[string]bool{}
			}
			if out[r.Source][root] == nil {
				out[r.Source][root] = map[string]bool{}
			}
			out[r.Source][root][field] = true
		}
	}
	for source, roots := range out {
		for root := range roots {
			if whole[source+":"+root] {
				return nil, fmt.Errorf("cannot combine whole and projected state: %s", root)
			}
		}
	}
	return out, nil
}

// Signature-only packages retain the exact selected native field declarations.
// Unselected backend callbacks may depend on packages outside the migrated
// closure. No trimmed struct declaration is emitted into the library.
func projectSignatureStructs(f *ast.File, roots map[string]map[string]bool) error {
	if len(roots) == 0 {
		return nil
	}
	specs := map[string]*ast.TypeSpec{}
	vars := map[string]*ast.ValueSpec{}
	for _, d := range f.Decls {
		if g, ok := d.(*ast.GenDecl); ok {
			for _, s := range g.Specs {
				switch s := s.(type) {
				case *ast.TypeSpec:
					specs[s.Name.Name] = s
				case *ast.ValueSpec:
					if g.Tok == token.VAR {
						for _, n := range s.Names {
							vars[n.Name] = s
						}
					}
				}
			}
		}
	}
	selected := map[string]map[string]bool{}
	for root, fields := range roots {
		v := vars[root]
		if v == nil {
			return fmt.Errorf("projected state variable not found: %s", root)
		}
		id, ok := v.Type.(*ast.Ident)
		if !ok || len(v.Names) != 1 {
			return fmt.Errorf("projected state requires an explicit named struct variable: %s", root)
		}
		spec := specs[id.Name]
		if spec == nil || spec.Assign.IsValid() || spec.TypeParams != nil {
			return fmt.Errorf("projected struct must be declared in source: %s", id.Name)
		}
		if _, ok := spec.Type.(*ast.StructType); !ok {
			return fmt.Errorf("projected state is not a struct: %s", root)
		}
		if selected[id.Name] == nil {
			selected[id.Name] = map[string]bool{}
		}
		for field := range fields {
			selected[id.Name][field] = true
		}
	}
	// A signature using the entire struct would require its complete type.
	for _, d := range f.Decls {
		if fn, ok := d.(*ast.FuncDecl); ok {
			bad := ""
			ast.Inspect(fn.Type, func(n ast.Node) bool {
				if id, ok := n.(*ast.Ident); ok && selected[id.Name] != nil {
					bad = id.Name
				}
				return true
			})
			if bad != "" {
				return fmt.Errorf("projected struct used by function signature: %s", bad)
			}
		}
	}
	for name, fields := range selected {
		st := specs[name].Type.(*ast.StructType)
		found := map[string]bool{}
		var keep []*ast.Field
		for _, f := range st.Fields.List {
			var names []*ast.Ident
			for _, n := range f.Names {
				if fields[n.Name] {
					names = append(names, n)
					found[n.Name] = true
				}
			}
			if len(names) > 0 {
				copy := *f
				copy.Names = names
				keep = append(keep, &copy)
			}
		}
		for field := range fields {
			if !found[field] {
				return fmt.Errorf("projected native field not found: %s.%s", name, field)
			}
		}
		st.Fields.List = keep
	}
	return nil
}

func externalStateMember(pkg *types.Package, name string) types.Object {
	root, field, projected := strings.Cut(name, ".")
	obj := pkg.Scope().Lookup(root)
	if obj == nil || !projected {
		return obj
	}
	member, _, _ := types.LookupFieldOrMethod(obj.Type(), true, pkg, field)
	v, ok := member.(*types.Var)
	if !ok || !v.IsField() {
		return nil
	}
	return member
}

func projectionRootID(e ast.Expr) *ast.Ident {
	for {
		if p, ok := e.(*ast.ParenExpr); ok {
			e = p.X
			continue
		}
		break
	}
	switch e := e.(type) {
	case *ast.Ident:
		return e
	case *ast.SelectorExpr:
		return e.Sel
	}
	return nil
}

func projectedStateBindings(files []*ast.File, info *types.Info, units []Unit) (map[ast.Node]string, map[types.Object]string, error) {
	rules := map[types.Object]map[string]string{}
	for _, u := range units {
		for _, r := range u.State {
			root, field, projected := strings.Cut(r.Variable, ".")
			if !projected {
				continue
			}
			pkg := strings.TrimPrefix(path.Dir(r.Source), "src/")
			for _, obj := range info.Uses {
				if obj != nil && obj.Pkg() != nil && obj.Pkg().Path() == pkg && obj.Name() == root && obj.Parent() == obj.Pkg().Scope() {
					if rules[obj] == nil {
						rules[obj] = map[string]string{}
					}
					if old := rules[obj][field]; old != "" && old != r.Field {
						return nil, nil, fmt.Errorf("conflicting projected state binding: %s", r.Variable)
					}
					rules[obj][field] = r.Field
				}
			}
		}
	}
	bindings := map[ast.Node]string{}
	covered := map[*ast.Ident]bool{}
	dependencies := map[types.Object]string{}
	for obj := range rules {
		dependencies[obj] = "projected state"
	}
	for _, f := range files {
		ast.Inspect(f, func(n ast.Node) bool {
			sel, ok := n.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			id := projectionRootID(sel.X)
			if id == nil {
				return true
			}
			fields := rules[info.Uses[id]]
			field := fields[sel.Sel.Name]
			if field == "" {
				return true
			}
			selection := info.Selections[sel]
			if selection == nil || selection.Kind() != types.FieldVal || len(selection.Index()) != 1 {
				return true
			}
			bindings[sel] = field
			covered[id] = true
			return true
		})
	}
	// Offsetof observes the enclosing native struct layout, which is not a
	// property preserved by moving a field into a compilation instance.
	for _, f := range files {
		var invalid bool
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			id := projectionRootID(call.Fun)
			if id == nil || info.Uses[id] != types.Unsafe.Scope().Lookup("Offsetof") {
				return true
			}
			for _, arg := range call.Args {
				ast.Inspect(arg, func(n ast.Node) bool { invalid = invalid || bindings[n] != ""; return !invalid })
			}
			return !invalid
		})
		if invalid {
			return nil, nil, fmt.Errorf("projected state cannot preserve unsafe.Offsetof")
		}
	}
	for id, obj := range info.Uses {
		if rules[obj] != nil && !covered[id] {
			return nil, nil, fmt.Errorf("projected state %s must use a reviewed direct field", obj.Name())
		}
	}
	return bindings, dependencies, nil
}

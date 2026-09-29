package port

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
)

func receiverType(fn *ast.FuncDecl) string {
	if fn.Recv == nil || len(fn.Recv.List) != 1 {
		return ""
	}
	t := fn.Recv.List[0].Type
	if p, ok := t.(*ast.StarExpr); ok {
		t = p.X
	}
	if id, ok := t.(*ast.Ident); ok {
		return id.Name
	}
	return ""
}
func ownedReceiver(rule *OwnerRule, name string) bool {
	for _, t := range rule.Types {
		if t == name {
			return true
		}
	}
	return false
}
func addOwnerFields(f *ast.File, rule *OwnerRule) error {
	if !token.IsIdentifier(rule.Context) || !token.IsIdentifier(rule.Field) {
		return fmt.Errorf("invalid owner rule")
	}
	found := map[string]bool{}
	for _, d := range f.Decls {
		g, ok := d.(*ast.GenDecl)
		if !ok || g.Tok != token.TYPE {
			continue
		}
		for _, sp := range g.Specs {
			t := sp.(*ast.TypeSpec)
			if !ownedReceiver(rule, t.Name.Name) {
				continue
			}
			st, ok := t.Type.(*ast.StructType)
			if !ok {
				return fmt.Errorf("owner target must be struct: %s", t.Name.Name)
			}
			existing := false
			for _, field := range st.Fields.List {
				for _, id := range field.Names {
					if id.Name == rule.Field {
						pointer, _ := field.Type.(*ast.StarExpr)
						var name *ast.Ident
						if pointer != nil {
							name, _ = pointer.X.(*ast.Ident)
						}
						if !rule.Existing || name == nil || name.Name != rule.Context {
							return fmt.Errorf("owner field collision or incompatible type: %s.%s", t.Name.Name, id.Name)
						}
						existing = true
					}
				}
			}
			if rule.Existing {
				if !existing {
					return fmt.Errorf("existing owner field not found: %s.%s", t.Name.Name, rule.Field)
				}
				found[t.Name.Name] = true
				continue
			}
			st.Fields.List = append(st.Fields.List, &ast.Field{Names: []*ast.Ident{{Name: rule.Field, NamePos: st.Fields.Closing}}, Type: &ast.StarExpr{Star: st.Fields.Closing, X: &ast.Ident{Name: rule.Context, NamePos: st.Fields.Closing}}})
			found[t.Name.Name] = true
		}
	}
	for _, name := range rule.Types {
		if !found[name] {
			return fmt.Errorf("owner type not selected: %s", name)
		}
	}
	return nil
}

func injectOwnedAllocations(node ast.Node, info *types.Info, rule *OwnerRule, ctx ast.Expr) error {
	owned := map[types.Type]bool{}
	for _, obj := range info.Defs {
		if typ, ok := obj.(*types.TypeName); ok && typ.Pkg() != nil && typ.Parent() == typ.Pkg().Scope() && ownedReceiver(rule, typ.Name()) {
			owned[typ.Type()] = true
		}
	}
	matches := func(expr ast.Expr) bool { return owned[info.TypeOf(expr)] }
	field := func() ast.Expr { return &ast.KeyValueExpr{Key: ast.NewIdent(rule.Field), Value: ctx} }
	replace := map[ast.Node]ast.Expr{}
	var err error
	ast.Inspect(node, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.CompositeLit:
			typ := info.TypeOf(n)
			if ptr, ok := typ.(*types.Pointer); ok {
				typ = ptr.Elem()
			}
			if !owned[typ] {
				break
			}
			if len(n.Elts) > 0 {
				if _, keyed := n.Elts[0].(*ast.KeyValueExpr); !keyed {
					st, ok := typ.Underlying().(*types.Struct)
					if !ok || len(n.Elts) != st.NumFields() {
						err = fmt.Errorf("owned literal no longer matches source fields")
						return false
					}
					for i, value := range n.Elts {
						name := st.Field(i).Name()
						if name == "_" {
							err = fmt.Errorf("unkeyed owned literal has blank field")
							return false
						}
						n.Elts[i] = &ast.KeyValueExpr{Key: &ast.Ident{Name: name, NamePos: value.Pos()}, Value: value}
					}
				}
			}
			present := false
			for _, e := range n.Elts {
				if _, ok := e.(*ast.KeyValueExpr); !ok {
					err = fmt.Errorf("unkeyed literal of owned type")
					return false
				}
				if id, ok := e.(*ast.KeyValueExpr).Key.(*ast.Ident); ok && id.Name == rule.Field {
					present = true
				}
			}
			if !present {
				n.Elts = append(n.Elts, field())
			}
		case *ast.ValueSpec:
			if n.Type != nil && len(n.Values) == 0 && matches(n.Type) {
				for range n.Names {
					n.Values = append(n.Values, &ast.CompositeLit{Type: n.Type, Elts: []ast.Expr{field()}})
				}
				// The generated literal already carries its owner; traversing it
				// with pre-rewrite type information cannot reveal new allocations.
				return false
			}
		case *ast.CallExpr:
			id, ok := n.Fun.(*ast.Ident)
			if !ok || len(n.Args) != 1 {
				break
			}
			builtin, ok := info.Uses[id].(*types.Builtin)
			if !ok || builtin.Name() != "new" || !matches(n.Args[0]) {
				break
			}
			replace[n] = &ast.UnaryExpr{Op: token.AND, X: &ast.CompositeLit{Type: n.Args[0], Elts: []ast.Expr{field()}}}
		}
		return true
	})
	if err != nil {
		return err
	}
	rewriteExprs(node, replace)
	return nil
}

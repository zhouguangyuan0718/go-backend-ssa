package port

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"strings"
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
			if rule.Embedded[t.Name.Name] != "" {
				if existing {
					return fmt.Errorf("embedded owner shadows existing field: %s", t.Name.Name)
				}
				found[t.Name.Name] = true
				continue
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
	paths := map[types.Type]string{}
	for _, obj := range info.Defs {
		if typ, ok := obj.(*types.TypeName); ok && typ.Pkg() != nil && typ.Parent() == typ.Pkg().Scope() && ownedReceiver(rule, typ.Name()) {
			owned[typ.Type()] = true
			paths[typ.Type()] = rule.Embedded[typ.Name()]
		}
	}
	matches := func(expr ast.Expr) bool { return owned[info.TypeOf(expr)] }
	field := func() ast.Expr { return &ast.KeyValueExpr{Key: ast.NewIdent(rule.Field), Value: ctx} }
	// The original type information establishes the value-embedding path. New
	// literals initialize the single field in its declaring base, never a shadow.
	bind := func(lit *ast.CompositeLit, typ types.Type) error {
		return bindEmbeddedOwner(lit, typ, paths[typ], rule.Field, field())
	}
	replace := map[ast.Node]ast.Expr{}
	var err error
	ast.Inspect(node, func(n ast.Node) bool {
		if err != nil {
			return false
		}
		switch n := n.(type) {
		case *ast.CompositeLit:
			typ := info.TypeOf(n)
			if ptr, ok := typ.(*types.Pointer); ok {
				typ = ptr.Elem()
			}
			if !owned[typ] {
				break
			}
			if err = bind(n, typ); err != nil {
				return false
			}
		case *ast.ValueSpec:
			if n.Type != nil && len(n.Values) == 0 && matches(n.Type) {
				for range n.Names {
					lit := &ast.CompositeLit{Type: n.Type}
					if err = bind(lit, info.TypeOf(n.Type)); err != nil {
						return false
					}
					n.Values = append(n.Values, lit)
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
			lit := &ast.CompositeLit{Type: n.Args[0]}
			if err = bind(lit, info.TypeOf(n.Args[0])); err != nil {
				return false
			}
			replace[n] = &ast.UnaryExpr{Op: token.AND, X: lit}
		}
		return true
	})
	if err != nil {
		return err
	}
	rewriteExprs(node, replace)
	return nil
}

// bindEmbeddedOwner follows only direct, non-pointer embedded fields. This is
// the native IR miniNode/miniExpr/miniStmt pattern. Source drift or an arbitrary
// expression supplying a base requires review rather than silently rehoming it.
func bindEmbeddedOwner(lit *ast.CompositeLit, typ types.Type, path, field string, value ast.Expr) error {
	st, ok := typ.Underlying().(*types.Struct)
	if !ok {
		return fmt.Errorf("owner target must be struct")
	}
	if len(lit.Elts) > 0 {
		if _, keyed := lit.Elts[0].(*ast.KeyValueExpr); !keyed {
			if len(lit.Elts) != st.NumFields() {
				return fmt.Errorf("owned literal no longer matches source fields")
			}
			for i, e := range lit.Elts {
				name := st.Field(i).Name()
				if name == "_" {
					return fmt.Errorf("unkeyed owned literal has blank field")
				}
				lit.Elts[i] = &ast.KeyValueExpr{Key: &ast.Ident{Name: name, NamePos: e.Pos()}, Value: e}
			}
		}
	}
	key, rest, _ := strings.Cut(path, ".")
	if path == "" {
		key = field
	}
	var slot *ast.KeyValueExpr
	for _, e := range lit.Elts {
		kv, ok := e.(*ast.KeyValueExpr)
		if !ok {
			return fmt.Errorf("unkeyed literal of owned type")
		}
		if id, ok := kv.Key.(*ast.Ident); ok && id.Name == key {
			slot = kv
		}
	}
	if path == "" {
		if slot == nil {
			lit.Elts = append(lit.Elts, value)
		}
		return nil
	}
	var embedded *types.Var
	for i := 0; i < st.NumFields(); i++ {
		f := st.Field(i)
		if f.Name() == key {
			embedded = f
			break
		}
	}
	if embedded == nil || !embedded.Embedded() {
		return fmt.Errorf("embedded owner path is not an embedded field: %s", key)
	}
	named, ok := embedded.Type().(*types.Named)
	if !ok || named.Obj().Pkg() != embedded.Pkg() {
		return fmt.Errorf("embedded owner requires a local named value: %s", key)
	}
	if slot == nil {
		slot = &ast.KeyValueExpr{Key: ast.NewIdent(key), Value: &ast.CompositeLit{Type: ast.NewIdent(named.Obj().Name())}}
		lit.Elts = append(lit.Elts, slot)
	}
	child, ok := slot.Value.(*ast.CompositeLit)
	if !ok {
		return fmt.Errorf("embedded owner initialization requires a literal: %s", key)
	}
	return bindEmbeddedOwner(child, embedded.Type(), rest, field, value)
}

func validateEmbeddedOwners(info *types.Info, rules []*OwnerRule) error {
	for _, rule := range rules {
		for _, obj := range info.Defs {
			name, ok := obj.(*types.TypeName)
			if !ok || name.Pkg() == nil || name.Parent() != name.Pkg().Scope() {
				continue
			}
			path := rule.Embedded[name.Name()]
			if path == "" {
				continue
			}
			typ := name.Type()
			for _, key := range strings.Split(path, ".") {
				st, ok := typ.Underlying().(*types.Struct)
				if !ok {
					return fmt.Errorf("invalid embedded owner struct: %s", key)
				}
				var field *types.Var
				for i := 0; i < st.NumFields(); i++ {
					if st.Field(i).Name() == key {
						field = st.Field(i)
						break
					}
				}
				if field == nil || !field.Embedded() {
					return fmt.Errorf("embedded owner path is not an embedded field: %s", key)
				}
				named, ok := field.Type().(*types.Named)
				if !ok || named.Obj().Pkg() != name.Pkg() {
					return fmt.Errorf("embedded owner requires a local named value: %s", key)
				}
				typ = named
			}
			leaf := typ.(*types.Named).Obj().Name()
			found := false
			for _, base := range rules {
				if ownedReceiver(base, leaf) && base.Embedded[leaf] == "" && base.Field == rule.Field && base.Context == rule.Context {
					found = true
					break
				}
			}
			if !found {
				return fmt.Errorf("embedded owner path does not end at a declared owner: %s", path)
			}
		}
	}
	return nil
}

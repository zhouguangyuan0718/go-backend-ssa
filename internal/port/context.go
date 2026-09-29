package port

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"path"
	"reflect"
	"strings"
)

// rewritePackageContext resolves state and calls by object identity across all
// selected files. Initialization helpers stay in their original files, so file
// scoped import aliases retain their meaning. The constructor follows InitOrder,
// then runs reviewed init bodies in lexical file/declaration order.
func rewritePackageContext(fs *token.FileSet, files []*ast.File, info *types.Info, units []Unit) error {
	var context *ContextRule
	first := -1
	states := map[types.Object]string{}
	functions := map[types.Object]string{}
	external := map[types.Object]string{}
	var owners []*OwnerRule
	for i, u := range units {
		if u.Owner != nil {
			owners = append(owners, u.Owner)
		}
		if u.Context == nil {
			continue
		}
		c := u.Context
		if first < 0 {
			first, context = i, c
		}
		if c.Type != context.Type || c.Constructor != context.Constructor {
			return fmt.Errorf("inconsistent package context")
		}
		for _, name := range []string{c.Type, c.Constructor} {
			if !token.IsIdentifier(name) {
				return fmt.Errorf("invalid context identifier %s", name)
			}
		}
		for id, obj := range info.Defs {
			if field, ok := c.Fields[id.Name]; ok && obj != nil && obj.Pkg() != nil && obj.Parent() == obj.Pkg().Scope() {
				if !token.IsIdentifier(field) {
					return fmt.Errorf("invalid context field %s", field)
				}
				states[obj] = field
			}
		}
		for _, call := range u.Calls {
			pkg := strings.TrimPrefix(path.Dir(call.Source), "src/")
			for _, obj := range info.Uses {
				if obj != nil && obj.Pkg() != nil && obj.Pkg().Path() == pkg && obj.Name() == call.Function && obj.Parent() == obj.Pkg().Scope() {
					if previous := external[obj]; previous != "" && previous != call.Method {
						return fmt.Errorf("conflicting external binding")
					}
					external[obj] = call.Method
				}
			}
		}
		for _, d := range files[i].Decls {
			if fn, ok := d.(*ast.FuncDecl); ok && fn.Recv == nil && fn.Name.Name != "init" {
				functions[info.Defs[fn.Name]] = fn.Name.Name
			}
		}
	}
	if context == nil {
		return nil
	}
	for _, owner := range owners {
		if owner.Context != context.Type {
			return fmt.Errorf("owner/context mismatch")
		}
	}
	for _, f := range files {
		collision := false
		ast.Inspect(f, func(n ast.Node) bool {
			if id, ok := n.(*ast.Ident); ok && (strings.HasPrefix(id.Name, "syncgo") || id.Name == context.Type || id.Name == context.Constructor) {
				collision = true
			}
			return true
		})
		if collision {
			return fmt.Errorf("context identifier collision")
		}
	}
	const recv = "syncgoCtx"
	replacements := func(ctx ast.Expr) map[*ast.Ident]ast.Expr {
		result := map[*ast.Ident]ast.Expr{}
		for id, obj := range info.Uses {
			name := states[obj]
			if name == "" {
				name = functions[obj]
			}
			if name == "" {
				name = external[obj]
			}
			if name != "" {
				result[id] = &ast.SelectorExpr{X: positionedContext(ctx, id.Pos()), Sel: &ast.Ident{Name: name, NamePos: id.Pos()}}
			}
		}
		return result
	}
	rewrite := func(node ast.Node, ctx ast.Expr) error {
		// Imported references are selectors, not bare identifiers. Match the
		// resolved member object, so local values with the same spelling survive.
		selectors := map[ast.Node]ast.Expr{}
		ast.Inspect(node, func(n ast.Node) bool {
			if sel, ok := n.(*ast.SelectorExpr); ok {
				if name := external[info.Uses[sel.Sel]]; name != "" {
					selectors[sel] = &ast.SelectorExpr{X: positionedContext(ctx, sel.Pos()), Sel: &ast.Ident{Name: name, NamePos: sel.Sel.Pos()}}
				}
			}
			return true
		})
		rewriteExprs(node, selectors)
		rewriteNodes(node, replacements(ctx))
		for _, owner := range owners {
			if err := injectOwnedAllocations(node, info, owner, ctx); err != nil {
				return err
			}
		}
		return nil
	}
	// Save initializer expressions before declaration rewriting changes their roots.
	helpers := make([]string, len(files))
	var calls []string
	previous := -1
	for _, entry := range info.InitOrder {
		if len(entry.Lhs) != 1 || states[entry.Lhs[0]] == "" {
			return fmt.Errorf("unsupported state initialization")
		}
		index := -1
		for i, f := range files {
			if entry.Lhs[0].Pos() >= f.Pos() && entry.Lhs[0].Pos() < f.End() {
				index = i
				break
			}
		}
		if index < 0 || units[index].Context == nil {
			return fmt.Errorf("initializer missing context source")
		}
		if index != previous {
			if previous >= 0 {
				helpers[previous] += "}\n"
			}
			name := fmt.Sprintf("syncgoInitVars%d", len(calls))
			helpers[index] += fmt.Sprintf("\nfunc (%s *%s) %s() {\n", recv, context.Type, name)
			calls = append(calls, name)
			previous = index
		}
		expr := &ast.ParenExpr{X: entry.Rhs}
		if err := rewrite(expr, ast.NewIdent(recv)); err != nil {
			return err
		}
		helpers[index] += fmt.Sprintf("%s.%s = %s\n", recv, states[entry.Lhs[0]], nodeText(fs, expr.X))
	}
	if previous >= 0 {
		helpers[previous] += "}\n"
	}
	for i, f := range files {
		if units[i].Context == nil {
			continue
		}
		var decls []ast.Decl
		for _, d := range f.Decls {
			if g, ok := d.(*ast.GenDecl); ok && g.Tok == token.VAR {
				continue
			}
			fn, ok := d.(*ast.FuncDecl)
			if !ok {
				decls = append(decls, d)
				continue
			}
			key := fn.Name.Name
			if fn.Recv != nil {
				key = receiverType(fn) + "." + key
			}
			check := units[i].Checks[key]
			ctx := ast.Expr(ast.NewIdent(recv))
			var guard []ast.Stmt
			if fn.Recv != nil {
				needs := check != ""
				refs := replacements(ctx)
				ast.Inspect(fn, func(n ast.Node) bool {
					if id, ok := n.(*ast.Ident); ok && refs[id] != nil {
						needs = true
					}
					return true
				})
				// Also bind methods that only allocate owned objects.
				needs = needs || hasOwnedAllocation(fn, info, owners)
				if !needs {
					decls = append(decls, d)
					continue
				}
				var owner *OwnerRule
				for _, o := range owners {
					if ownedReceiver(o, receiverType(fn)) {
						owner = o
						break
					}
				}
				if owner == nil || len(fn.Recv.List[0].Names) != 1 {
					return fmt.Errorf("receiver context requires an owner rule: %s", fn.Name.Name)
				}
				receiver := fn.Recv.List[0].Names[0].Name
				// A value receiver has an owner too, but cannot be compared to nil.
				condition := receiver + "." + owner.Field + " == nil"
				if _, pointer := fn.Recv.List[0].Type.(*ast.StarExpr); pointer {
					condition = receiver + " == nil || " + condition
				}
				var err error
				guard, err = contextStatements("if "+condition+" { panic(\"receiver has no compilation owner\") }", fn.Body.Lbrace)
				if err != nil {
					return err
				}
				ctx = &ast.SelectorExpr{X: ast.NewIdent(receiver), Sel: ast.NewIdent(owner.Field)}
			} else {
				var err error
				guard, err = contextStatements("if syncgoCtx == nil { panic(\"uninitialized compilation\") }", fn.Body.Lbrace)
				if err != nil {
					return err
				}
				if fn.Name.Name == "init" {
					if !units[i].Context.Init {
						return fmt.Errorf("package init requires an explicit adaptation rule")
					}
					name := fmt.Sprintf("syncgoInit%d", len(calls))
					fn.Name = ast.NewIdent(name)
					calls = append(calls, name)
				}
				fn.Recv = &ast.FieldList{List: []*ast.Field{{Names: []*ast.Ident{ast.NewIdent(recv)}, Type: &ast.StarExpr{X: ast.NewIdent(context.Type)}}}}
			}
			if err := rewrite(fn.Body, ctx); err != nil {
				return err
			}
			checks, err := ownedArgumentChecks(fn, info, owners, nodeText(fs, ctx))
			if err != nil {
				return err
			}
			if check != "" {
				args := []string{}
				for _, field := range fn.Type.Params.List {
					if _, variadic := field.Type.(*ast.Ellipsis); variadic {
						return fmt.Errorf("entry check on variadic function requires explicit adaptation: %s", key)
					}
					if len(field.Names) == 0 {
						return fmt.Errorf("unnamed checked parameter: %s", key)
					}
					for _, name := range field.Names {
						if name.Name == "_" {
							return fmt.Errorf("blank checked parameter: %s", key)
						}
						args = append(args, name.Name)
					}
				}
				stmt, err := contextStatements(nodeText(fs, ctx)+"."+check+"("+strings.Join(args, ",")+")", fn.Body.Lbrace)
				if err != nil {
					return err
				}
				checks = append(checks, stmt...)
			}
			// Preserve an upstream early nil-receiver return before consulting
			// an owner. In particular Type.IsUntyped(nil) must remain false.
			prefix := 0
			if leadingNilReturn(fn, info) {
				prefix = 1
			}
			body := append([]ast.Stmt(nil), fn.Body.List[:prefix]...)
			body = append(body, guard...)
			body = append(body, checks...)
			fn.Body.List = append(body, fn.Body.List[prefix:]...)
			decls = append(decls, fn)
		}
		comments := ast.NewCommentMap(fs, f, f.Comments)
		f.Decls = decls
		f.Comments = comments.Filter(f).Comments()
	}
	constructor := fmt.Sprintf("\nfunc %s() *%s {\n%s := &%s{}\n", context.Constructor, context.Type, recv, context.Type)
	for _, name := range calls {
		constructor += recv + "." + name + "()\n"
	}
	constructor += "return " + recv + "\n}\n"
	helpers[first] += constructor
	for i, code := range helpers {
		if code == "" {
			continue
		}
		extra, err := parser.ParseFile(fs, units[i].Source+".context", "package "+files[i].Name.Name+"\n"+code, parser.ParseComments)
		if err != nil {
			return err
		}
		files[i].Decls = append(files[i].Decls, extra.Decls...)
	}
	return nil
}

func contextStatements(code string, pos token.Pos) ([]ast.Stmt, error) {
	f, err := parser.ParseFile(token.NewFileSet(), "checks.go", "package p;func f(){"+code+"}", parser.SkipObjectResolution)
	if err != nil {
		return nil, err
	}
	// Positions from the temporary parse must not be interpreted in the
	// upstream FileSet; they otherwise introduce spurious line breaks.
	setTokenPositions(reflect.ValueOf(f), pos)
	return f.Decls[0].(*ast.FuncDecl).Body.List, nil
}

func ownedTypeRule(t types.Type, owners []*OwnerRule, info *types.Info) *OwnerRule {
	named, ok := t.(*types.Named)
	if !ok {
		return nil
	}
	obj := named.Obj()
	if obj.Pkg() == nil || obj.Parent() != obj.Pkg().Scope() {
		return nil
	}
	selected := false
	for _, def := range info.Defs {
		if def == obj {
			selected = true
			break
		}
	}
	if !selected {
		return nil
	}
	for _, o := range owners {
		if ownedReceiver(o, obj.Name()) {
			return o
		}
	}
	return nil
}

func hasOwnedAllocation(node ast.Node, info *types.Info, owners []*OwnerRule) bool {
	found := false
	ast.Inspect(node, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.CompositeLit:
			found = found || ownedTypeRule(info.TypeOf(n.Type), owners, info) != nil
		case *ast.ValueSpec:
			if n.Type != nil && len(n.Values) == 0 {
				found = found || ownedTypeRule(info.TypeOf(n.Type), owners, info) != nil
			}
		case *ast.CallExpr:
			if id, ok := n.Fun.(*ast.Ident); ok && len(n.Args) == 1 {
				if b, ok := info.Uses[id].(*types.Builtin); ok && b.Name() == "new" {
					found = found || ownedTypeRule(info.TypeOf(n.Args[0]), owners, info) != nil
				}
			}
		}
		return true
	})
	return found
}

// Match *owned-type and []*owned-type parameters. Invalid graph composition is
// rejected before any upstream mutation. Slice storage keeps upstream ownership
// semantics: callers must not mutate it to insert foreign nodes after the call.
func ownedArgumentChecks(fn *ast.FuncDecl, info *types.Info, owners []*OwnerRule, ctx string) ([]ast.Stmt, error) {
	var code strings.Builder
	for _, field := range fn.Type.Params.List {
		t := info.TypeOf(field.Type)
		slice, isSlice := t.(*types.Slice)
		if isSlice {
			t = slice.Elem()
		}
		pointer, ok := t.(*types.Pointer)
		if !ok {
			continue
		}
		owner := ownedTypeRule(pointer.Elem(), owners, info)
		if owner == nil {
			continue
		}
		for _, name := range field.Names {
			arg := name.Name
			if arg == "_" {
				continue
			}
			if isSlice {
				fmt.Fprintf(&code, "for _, syncgoArg := range %s {\n", arg)
				arg = "syncgoArg"
			}
			fmt.Fprintf(&code, "if %s == nil || %s == nil || %s.%s != %s { panic(\"argument belongs to another or uninitialized compilation\") }\n", ctx, arg, arg, owner.Field, ctx)
			if isSlice {
				code.WriteString("}\n")
			}
		}
	}
	return contextStatements(code.String(), fn.Body.Lbrace)
}

func positionedContext(expr ast.Expr, pos token.Pos) ast.Expr {
	switch e := expr.(type) {
	case *ast.Ident:
		return &ast.Ident{Name: e.Name, NamePos: pos}
	case *ast.SelectorExpr:
		return &ast.SelectorExpr{X: positionedContext(e.X, pos), Sel: &ast.Ident{Name: e.Sel.Name, NamePos: pos}}
	default:
		panic("unexpected context expression")
	}
}

func setTokenPositions(v reflect.Value, pos token.Pos) {
	if !v.IsValid() {
		return
	}
	if v.Type() == reflect.TypeOf(token.NoPos) && v.CanSet() {
		if v.Int() != 0 {
			v.SetInt(int64(pos))
		}
		return
	}
	switch v.Kind() {
	case reflect.Interface, reflect.Pointer:
		if !v.IsNil() {
			setTokenPositions(v.Elem(), pos)
		}
	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			setTokenPositions(v.Field(i), pos)
		}
	case reflect.Slice:
		for i := 0; i < v.Len(); i++ {
			setTokenPositions(v.Index(i), pos)
		}
	}
}

// Recognize only the reviewed, side-effect-free form "if recv == nil { return
// constants-or-nil }". More complex nil handling still requires explicit adaptation.
func leadingNilReturn(fn *ast.FuncDecl, info *types.Info) bool {
	if fn.Recv == nil || len(fn.Recv.List) != 1 || len(fn.Recv.List[0].Names) != 1 || len(fn.Body.List) == 0 {
		return false
	}
	receiver := info.Defs[fn.Recv.List[0].Names[0]]
	if receiver == nil {
		return false
	} // A newly injected Context receiver.
	branch, ok := fn.Body.List[0].(*ast.IfStmt)
	if !ok || branch.Init != nil || branch.Else != nil || len(branch.Body.List) != 1 {
		return false
	}
	ret, ok := branch.Body.List[0].(*ast.ReturnStmt)
	if !ok || len(ret.Results) == 0 {
		return false
	}
	for _, result := range ret.Results {
		if info.Types[result].Value != nil {
			continue
		}
		if id, ok := result.(*ast.Ident); ok && info.Uses[id] == types.Universe.Lookup("nil") {
			continue
		}
		return false
	}
	cond, ok := branch.Cond.(*ast.BinaryExpr)
	if !ok || cond.Op != token.EQL {
		return false
	}
	a, aOK := cond.X.(*ast.Ident)
	b, bOK := cond.Y.(*ast.Ident)
	if !aOK || !bOK {
		return false
	}
	nilObject := types.Universe.Lookup("nil")
	return info.Uses[a] == receiver && info.Uses[b] == nilObject || info.Uses[b] == receiver && info.Uses[a] == nilObject
}

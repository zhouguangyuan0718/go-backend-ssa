package port

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"path"
	"reflect"
	"sort"
	"strings"
)

// rewritePackageContext resolves state and calls by object identity across all
// selected files. Initialization helpers stay in their original files, so file
// scoped import aliases retain their meaning. The constructor follows InitOrder,
// then runs reviewed init bodies in lexical file/declaration order.
func rewritePackageContext(fs *token.FileSet, files []*ast.File, info *types.Info, units []Unit, externalRecovering ...map[types.Object]bool) error {
	recovering := map[types.Object]bool{}
	if len(externalRecovering) > 0 {
		recovering = externalRecovering[0]
	}
	var context *ContextRule
	first := -1
	states := map[types.Object]string{}
	functions := map[types.Object]string{}
	selective := map[types.Object]bool{}
	stateless := map[types.Object]bool{}
	external := map[types.Object]string{}
	retainedCalls := map[types.Object]bool{}
	requestedRetained := map[string]bool{}
	matchedRetained := map[string]bool{}
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
		for _, state := range u.State {
			pkg := strings.TrimPrefix(path.Dir(state.Source), "src/")
			for _, obj := range info.Uses {
				if obj != nil && obj.Pkg() != nil && obj.Pkg().Path() == pkg && obj.Name() == state.Variable && obj.Parent() == obj.Pkg().Scope() {
					if previous := external[obj]; previous != "" && previous != state.Field {
						return fmt.Errorf("conflicting external state binding")
					}
					external[obj] = state.Field
				}
			}
		}
		for _, call := range u.Calls {
			if call.KeepDefinition {
				requestedRetained[call.Source+":"+call.Function] = true
			}
			pkg := strings.TrimPrefix(path.Dir(call.Source), "src/")
			for _, obj := range info.Uses {
				if obj != nil && obj.Pkg() != nil && obj.Pkg().Path() == pkg && obj.Name() == call.Function && obj.Parent() == obj.Pkg().Scope() {
					if previous := external[obj]; previous != "" && previous != call.Method {
						return fmt.Errorf("conflicting external binding")
					}
					external[obj] = call.Method
					if call.KeepDefinition {
						retainedCalls[obj] = true
						matchedRetained[call.Source+":"+call.Function] = true
					}
				}
			}
		}
		for _, resource := range u.HostResources {
			for _, obj := range info.Uses {
				if hostResourceFactory(obj, resource) {
					if previous := external[obj]; previous != "" && previous != resource.Method {
						return fmt.Errorf("conflicting host resource binding")
					}
					external[obj] = resource.Method
				}
			}
		}
		for _, call := range u.HostCalls {
			for _, object := range info.Uses {
				if hostCallFunction(object, call) {
					if previous := external[object]; previous != "" && previous != call.Method {
						return fmt.Errorf("conflicting host call binding")
					}
					external[object] = call.Method
				}
			}
		}
		for _, d := range files[i].Decls {
			if fn, ok := d.(*ast.FuncDecl); ok {
				selective[info.Defs[fn.Name]] = c.Selective
			}
		}
		for _, name := range u.StatelessFunctions {
			found := false
			for _, decl := range files[i].Decls {
				if fn, ok := decl.(*ast.FuncDecl); ok && fn.Recv == nil && fn.Name.Name == name {
					object := info.Defs[fn.Name]
					selective[object] = true
					stateless[object] = true
					found = true
				}
			}
			if !found {
				return fmt.Errorf("stateless function not selected: %s", name)
			}
		}
		renamed := map[string]bool{}
		for _, d := range files[i].Decls {
			if fn, ok := d.(*ast.FuncDecl); ok && fn.Recv == nil && fn.Name.Name != "init" && !(c.Existing && fn.Name.Name == c.Constructor) {
				name := fn.Name.Name
				if target := u.ContextMethods[name]; target != "" {
					renamed[name] = true
					name = target
				}
				functions[info.Defs[fn.Name]] = name
			}
		}
		for name := range u.ContextMethods {
			if !renamed[name] {
				return fmt.Errorf("renamed context function not selected: %s", name)
			}
		}
	}
	if context == nil {
		return nil
	}
	replaceConstantContextLengths(files, info, states)
	groups, err := contextInitializerGroups(files, info, units, states)
	if err != nil {
		return err
	}
	fieldGroups := map[string]string{}
	for object, field := range states {
		for i, file := range files {
			if object.Pos() >= file.Pos() && object.Pos() < file.End() {
				fieldGroups[field] = groups[i]
			}
		}
	}
	projections, projectedDependencies, err := projectedStateBindings(files, info, units)
	if err != nil {
		return err
	}
	literals, err := literalBindings(files, info, units)
	if err != nil {
		return err
	}
	derived, err := contextSourceFunctions(files, info, units)
	if err != nil {
		return err
	}
	for obj := range derived {
		delete(functions, obj)
	}
	if err := validateEmbeddedOwners(info, owners); err != nil {
		return err
	}
	for key := range requestedRetained {
		if !matchedRetained[key] {
			return fmt.Errorf("retained native call is unused: %s", key)
		}
	}
	for obj := range retainedCalls {
		if functions[obj] == "" {
			return fmt.Errorf("retained call requires a selected local function: %s", obj.Name())
		}
		selective[obj] = false
	}
	for obj := range external {
		if retainedCalls[obj] {
			continue
		}
		delete(functions, obj) // Explicit bindings take precedence over generated methods.
	}
	traceBindings := map[types.Object]string{}
	if context.RedirectTrace || context.RedirectExit || context.RedirectReadFile {
		traceBindings = hostReferences(info, context.RedirectTrace, context.RedirectExit, context.RedirectReadFile)
		for obj, method := range traceBindings {
			external[obj] = method
		}
	}
	binding := func(ctx ast.Expr, obj types.Object, name string, pos token.Pos) ast.Expr {
		var result ast.Expr = positionedContext(ctx, pos)
		for _, part := range strings.Split(name, ".") {
			result = &ast.SelectorExpr{X: result, Sel: &ast.Ident{Name: part, NamePos: pos}}
		}
		if traceBindings[obj] == "traceWriter" {
			result = &ast.CallExpr{Fun: result}
		}
		return result
	}
	contextExternal := map[types.Object]string{}
	for obj, field := range external {
		contextExternal[obj] = field
	}
	for obj, field := range projectedDependencies {
		contextExternal[obj] = field
	}
	boundMethods, err := methodCallBindings(files, info, units)
	if err != nil {
		return err
	}
	for obj, field := range boundMethods {
		contextExternal[obj] = field
	}
	externalMethods, err := externalContextMethods(files, info, units)
	if err != nil {
		return err
	}
	for obj, call := range externalMethods {
		contextExternal[obj] = call.Field
	}
	parameterMethods, err := contextParameterMethods(files, info, units)
	if err != nil {
		return err
	}
	{
		candidates := map[types.Object]string{}
		for obj, name := range functions {
			candidates[obj] = name
		}
		for obj := range parameterMethods {
			candidates[obj] = obj.Name()
		}
		needed := statefulFunctions(files, info, candidates, states, contextExternal, owners, units, literals)
		for obj := range functions {
			if stateless[obj] && needed[obj] != "" {
				return fmt.Errorf("stateless function uses compilation state: %s", obj.Name())
			}
			if selective[obj] && needed[obj] == "" {
				if functions[obj] != obj.Name() {
					return fmt.Errorf("renamed function does not need context: %s", obj.Name())
				}
				delete(functions, obj)
			}
		}
		for obj := range parameterMethods {
			if selective[obj] && needed[obj] == "" {
				delete(parameterMethods, obj)
			}
		}
	}
	for _, owner := range owners {
		if owner.Context != context.Type {
			return fmt.Errorf("owner/context mismatch")
		}
	}
	for _, f := range files {
		collision := false
		ast.Inspect(f, func(n ast.Node) bool {
			if id, ok := n.(*ast.Ident); ok && (strings.HasPrefix(id.Name, "syncgo") || !context.Existing && (id.Name == context.Type || id.Name == context.Constructor)) {
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
				name = external[obj]
			}
			if name == "" {
				name = functions[obj]
			}
			if name != "" {
				result[id] = binding(ctx, obj, name, id.Pos())
			}
		}
		return result
	}
	rewrite := func(node ast.Node, ctx ast.Expr) error {
		// Imported references are selectors, not bare identifiers. Match the
		// resolved member object, so local values with the same spelling survive.
		selectors := map[ast.Node]ast.Expr{}
		ast.Inspect(node, func(n ast.Node) bool {
			if field := projections[n]; field != "" {
				selectors[n] = binding(ctx, nil, field, n.Pos())
				return false
			}
			if sel, ok := n.(*ast.SelectorExpr); ok {
				if name := external[info.Uses[sel.Sel]]; name != "" {
					selectors[sel] = binding(ctx, info.Uses[sel.Sel], name, sel.Pos())
				}
			}
			return true
		})
		rewriteBoundMethodCalls(node, info, boundMethods, ctx)
		rewriteExprs(node, selectors)
		rewriteNodes(node, replacements(ctx))
		for _, owner := range owners {
			if err := injectOwnedAllocations(node, info, owner, ctx); err != nil {
				return err
			}
		}
		var bound []ast.Node
		ast.Inspect(node, func(n ast.Node) bool {
			if literals[n] != "" {
				bound = append(bound, n)
			}
			return true
		})
		// Inner literals first, preserving every initializer and its evaluation order.
		for i := len(bound) - 1; i >= 0; i-- {
			lit := bound[i]
			if v, ok := lit.(*ast.ValueSpec); ok {
				for range v.Names {
					v.Values = append(v.Values, &ast.CallExpr{Fun: binding(ctx, nil, literals[v], v.Pos()), Args: []ast.Expr{&ast.CompositeLit{Type: v.Type}}})
				}
				continue
			}
			rewriteExprs(node, map[ast.Node]ast.Expr{lit: &ast.CallExpr{Fun: binding(ctx, nil, literals[lit], lit.Pos()), Args: []ast.Expr{lit.(ast.Expr)}}})
		}
		return threadExternalMethodContexts(fs, files, node, info, externalMethods, parameterMethods, ctx, context.Type, recovering)
	}
	// Include receiver methods in the dependency closure: a pure helper is safe
	// during allocation, but an indirect read of moved state is not.
	allFunctions := map[types.Object]string{}
	for _, f := range files {
		for _, d := range f.Decls {
			if fn, ok := d.(*ast.FuncDecl); ok {
				allFunctions[info.Defs[fn.Name]] = fn.Name.Name
			}
		}
	}
	constructorDeps := statefulFunctions(files, info, allFunctions, states, contextExternal, owners, units, literals)
	unsafeValueInit := false
	for _, entry := range info.InitOrder {
		dependent := hasOwnedAllocation(entry.Rhs, info, owners) || hasBoundLiteral(entry.Rhs, literals)
		ast.Inspect(entry.Rhs, func(n ast.Node) bool {
			if id, ok := n.(*ast.Ident); ok {
				obj := info.Uses[id]
				dependent = dependent || states[obj] != "" || contextExternal[obj] != "" || constructorDeps[obj] != ""
			}
			return true
		})
		unsafeValueInit = unsafeValueInit || dependent
	}

	// Save initializer expressions before declaration rewriting changes their roots.
	helpers := make([]string, len(files))
	var calls []string
	callGroups := map[string]string{}
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
			callGroups[name] = groups[index]
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
	constructorFile := first
	constructorType := "func() *" + context.Type
	constructorArgs := ""
	valueContext := false

	for i, f := range files {
		// Local replacements apply even when their defining file needs no context.
		var retained []ast.Decl
		for _, d := range f.Decls {
			if fn, ok := d.(*ast.FuncDecl); ok && fn.Recv == nil && external[info.Defs[fn.Name]] != "" && !retainedCalls[info.Defs[fn.Name]] {
				continue
			}
			retained = append(retained, d)
		}
		f.Decls = retained
		if units[i].Context == nil {
			continue
		}
		var decls []ast.Decl
		for _, d := range f.Decls {
			if g, ok := d.(*ast.GenDecl); ok && g.Tok == token.VAR {
				for _, spec := range g.Specs {
					for _, id := range spec.(*ast.ValueSpec).Names {
						if states[info.Defs[id]] == "" {
							return fmt.Errorf("package variable has no context rule: %s", id.Name)
						}
					}
				}
				continue
			}
			fn, ok := d.(*ast.FuncDecl)
			if !ok {
				decls = append(decls, d)
				continue
			}
			if context.Existing && fn.Recv == nil && fn.Name.Name == context.Constructor {
				if context.Initialize {
					return fmt.Errorf("receiver initialization cannot wrap a native constructor")
				}
				sig := info.Defs[fn.Name].Type().(*types.Signature)
				var result types.Type
				if sig.Results().Len() == 1 {
					result = sig.Results().At(0).Type()
				}
				elem := result
				if pointer, ok := result.(*types.Pointer); ok {
					elem = pointer.Elem()
				} else {
					valueContext = true
				}
				if elem == nil || types.TypeString(elem, func(*types.Package) string { return "" }) != context.Type || sig.TypeParams().Len() != 0 {
					return fmt.Errorf("native context constructor must have signature func(...) %s or *%s", context.Type, context.Type)
				}
				constructorFile = i
				constructorType = nodeText(fs, fn.Type)
				// Names may be absent or blank in the original; the wrapper needs
				// stable argument names without changing the original declaration.
				params := make([]string, sig.Params().Len())
				for j := range params {
					params[j] = fmt.Sprintf("syncgoArg%d", j)
				}
				wrapper, err := parser.ParseExpr(constructorType)
				if err != nil {
					return err
				}
				j := 0
				for _, field := range wrapper.(*ast.FuncType).Params.List {
					if len(field.Names) == 0 {
						field.Names = []*ast.Ident{ast.NewIdent(params[j])}
						j++
					} else {
						for k := range field.Names {
							field.Names[k] = ast.NewIdent(params[j])
							j++
						}
					}
				}
				constructorType = nodeText(token.NewFileSet(), wrapper)
				constructorArgs = strings.Join(params, ",")
				if sig.Variadic() {
					constructorArgs += "..."
				}
				dependent := false
				refs := replacements(ast.NewIdent(recv))
				ast.Inspect(fn.Body, func(n ast.Node) bool {
					if id, ok := n.(*ast.Ident); ok {
						if refs[id] != nil {
							dependent = true
						}
						// A package call can read moved state indirectly (including
						// native receiver methods). Require an explicit adaptation
						// before allowing such calls during original allocation.
						if constructorDeps[info.Uses[id]] != "" {
							dependent = true
						}
					}
					return true
				})
				if dependent || hasOwnedAllocation(fn.Body, info, owners) || hasBoundLiteral(fn.Body, literals) {
					return fmt.Errorf("native context constructor depends on state before initialization")
				}
				fn.Name = &ast.Ident{Name: "syncgoNewContext", NamePos: fn.Name.Pos()}
				decls = append(decls, d)
				continue
			}
			if selective[info.Defs[fn.Name]] && fn.Recv == nil && fn.Name.Name != "init" && functions[info.Defs[fn.Name]] == "" && derived[info.Defs[fn.Name]].Resolver == "" {
				decls = append(decls, d)
				continue
			}
			key := fn.Name.Name
			if fn.Recv != nil {
				key = receiverType(fn) + "." + key
			}
			check := units[i].Checks[key]
			nullable := append([]string(nil), units[i].Nullable[key]...)
			ctx := ast.Expr(ast.NewIdent(recv))
			var guard []ast.Stmt
			if source := derived[info.Defs[fn.Name]]; source.Resolver != "" {
				var err error
				guard, err = contextStatements("syncgoCtx := "+source.Resolver+"("+source.Argument+"); if syncgoCtx == nil {panic(\"uninitialized compilation\")}", fn.Body.Lbrace)
				if err != nil {
					return err
				}
			} else if parameterMethods[info.Defs[fn.Name]] {
				for _, field := range fn.Type.Params.List {
					if len(field.Names) == 0 {
						field.Names = []*ast.Ident{{Name: "_", NamePos: field.Pos()}}
					}
				}
				fn.Type.Params.List = append([]*ast.Field{{Names: []*ast.Ident{{Name: recv, NamePos: fn.Type.Params.Opening}}, Type: &ast.StarExpr{Star: fn.Type.Params.Opening, X: &ast.Ident{Name: context.Type, NamePos: fn.Type.Params.Opening}}}}, fn.Type.Params.List...)
				var err error
				guard, err = contextStatements("if syncgoCtx == nil { panic(\"uninitialized compilation\") }", fn.Body.Lbrace)
				if err != nil {
					return err
				}
			} else if fn.Recv != nil {
				needs := check != "" || len(nullable) != 0
				refs := replacements(ctx)
				ast.Inspect(fn, func(n ast.Node) bool {
					if id, ok := n.(*ast.Ident); ok && (refs[id] != nil || parameterMethods[info.Uses[id]]) {
						needs = true
					}
					return true
				})
				// Also bind methods that only allocate owned objects.
				needs = needs || hasOwnedAllocation(fn, info, owners) || hasBoundLiteral(fn, literals)
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
				nativeReceiver := context.Existing && receiverType(fn) == context.Type
				if (!nativeReceiver && owner == nil) || len(fn.Recv.List[0].Names) != 1 {
					return fmt.Errorf("receiver context requires an owner rule: %s", key)
				}
				receiver := fn.Recv.List[0].Names[0].Name
				// A value receiver has an owner too, but cannot be compared to nil.
				condition := "false"
				if !nativeReceiver {
					condition = receiver + "." + owner.Field + " == nil"
				}
				if _, pointer := fn.Recv.List[0].Type.(*ast.StarExpr); pointer {
					condition = receiver + " == nil || " + condition
				}
				var err error
				guard, err = contextStatements("if "+condition+" { panic(\"receiver has no compilation owner\") }", fn.Body.Lbrace)
				if err != nil {
					return err
				}
				if nativeReceiver {
					ctx = ast.NewIdent(receiver)
				} else {
					ctx = &ast.SelectorExpr{X: ast.NewIdent(receiver), Sel: ast.NewIdent(owner.Field)}
				}
				for n, name := range nullable {
					if nativeReceiver && name == receiver {
						return fmt.Errorf("native context receiver cannot be nullable")
					}
					if name == receiver {
						if context.Initialize {
							return fmt.Errorf("nullable receiver requires a constructor without dependent inputs: %s", key)
						}
						if _, ok := fn.Recv.List[0].Type.(*ast.StarExpr); !ok {
							return fmt.Errorf("nullable receiver must be pointer: %s", key)
						}
						// Nil formatting has no owner. Give this invocation fresh
						// state, retaining the upstream formatter's nil semantics.
						guard, err = contextStatements("var syncgoCtx *"+context.Type+"; if "+receiver+" == nil { syncgoCtx = "+context.Constructor+"() } else { syncgoCtx = "+receiver+"."+owner.Field+" }; if syncgoCtx == nil { panic(\"receiver has no compilation owner\") }", fn.Body.Lbrace)
						if err != nil {
							return err
						}
						ctx = ast.NewIdent(recv)
						nullable = append(nullable[:n], nullable[n+1:]...)
						break
					}
				}
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
					callGroups[name] = groups[i]
				}
				if name := functions[info.Defs[fn.Name]]; name != "" && name != fn.Name.Name {
					fn.Name = &ast.Ident{Name: name, NamePos: fn.Name.Pos()}
				}
				fn.Recv = &ast.FieldList{List: []*ast.Field{{Names: []*ast.Ident{ast.NewIdent(recv)}, Type: &ast.StarExpr{X: ast.NewIdent(context.Type)}}}}
			}
			prefix := 0
			if count := units[i].ContextAfterGuards[key]; count != 0 {
				if err := checkContextGuardPrefix(fn, info, count); err != nil {
					return err
				}
				prefix = count
			} else if leadingNilReturn(fn, info) {
				prefix = 1
			}
			if err := rewrite(fn.Body, ctx); err != nil {
				return err
			}
			checks, err := ownedArgumentChecks(fn, info, owners, nodeText(fs, ctx), nullable, context.PreserveNilArgs)
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
			body := append([]ast.Stmt(nil), fn.Body.List[:prefix]...)
			body = append(body, guard...)
			body = append(body, checks...)
			fn.Body.List = append(body, fn.Body.List[prefix:]...)
			decls = append(decls, fn)
		}
		// Binding callbacks may add signature-type imports while declarations
		// are being rewritten. Keep those additions when installing the result.
		listed := map[*ast.ImportSpec]bool{}
		for _, d := range decls {
			if g, ok := d.(*ast.GenDecl); ok && g.Tok == token.IMPORT {
				for _, spec := range g.Specs {
					listed[spec.(*ast.ImportSpec)] = true
				}
			}
		}
		for _, im := range f.Imports {
			if !listed[im] {
				decls = append([]ast.Decl{&ast.GenDecl{Tok: token.IMPORT, TokPos: f.Name.End(), Specs: []ast.Spec{im}}}, decls...)
			}
		}
		comments := ast.NewCommentMap(fs, f, f.Comments)
		f.Decls = decls
		f.Comments = comments.Filter(f).Comments()
		// Synthesized fields can extend a removed initializer's token span
		// across the following declaration's Doc. Keep comments still attached
		// to retained AST nodes, while using the updated tree for other comments
		// (including trailing directives whose original expression was replaced).
		seen := map[*ast.CommentGroup]bool{}
		for _, group := range f.Comments {
			seen[group] = true
		}
		var attached []*ast.CommentGroup
		ast.Inspect(f, func(n ast.Node) bool {
			if group, ok := n.(*ast.CommentGroup); ok && !seen[group] {
				seen[group] = true
				attached = append(attached, group)
			}
			return true
		})
		f.Comments = append(f.Comments, attached...)
		sort.Slice(f.Comments, func(i, j int) bool { return f.Comments[i].Pos() < f.Comments[j].Pos() })
	}
	allocate := "&" + context.Type + "{}"
	if context.Existing {
		allocate = "syncgoNewContext(" + constructorArgs + ")"
	}
	constructor := "\nfunc " + context.Constructor + strings.TrimPrefix(constructorType, "func") + " {\n" + recv + " := " + allocate + "\n"
	if context.Initialize {
		constructor = "\nfunc (" + recv + " *" + context.Type + ") " + context.Constructor + "() {\nif " + recv + " == nil { panic(\"uninitialized compilation\") }\n"
	}
	// Source variables exist at their zero value before any initializer runs.
	// Bind value-typed owned state at that same point, including variables whose
	// initializer may inspect another variable through a method call.
	valueOwners := map[string]string{}
	for obj, field := range states {
		if owner := ownedTypeRule(obj.Type(), owners, info); owner != nil {
			valueOwners[field] = owner.Field
		}
	}
	if valueContext {
		// Returning a copied context cannot preserve back-pointers installed by
		// state initializers. Native value constructors are for plain state.
		for _, owner := range owners {
			if !owner.Existing {
				return fmt.Errorf("value context requires existing owner fields")
			}
		}
		if len(valueOwners) != 0 {
			return fmt.Errorf("value context cannot own value state")
		}
		for _, u := range units {
			if u.Context != nil && u.Context.Init {
				return fmt.Errorf("value context cannot run stateful init")
			}
		}
		if unsafeValueInit {
			return fmt.Errorf("value context initializer may retain context before copy")
		}

	}
	for _, field := range sortedKeys(valueOwners) {
		if fieldGroups[field] == "" {
			constructor += fmt.Sprintf("%s.%s.%s = %s\n", recv, field, valueOwners[field], recv)
		}
	}
	for _, name := range calls {
		if callGroups[name] == "" {
			constructor += recv + "." + name + "()\n"
		}
	}
	if !context.Initialize {
		constructor += "return " + recv + "\n"
	}
	constructor += "}\n"
	helpers[constructorFile] += constructor
	groupFiles := map[string]int{}
	for i, group := range groups {
		if group != "" {
			if _, exists := groupFiles[group]; !exists {
				groupFiles[group] = i
			}
		}
	}
	for _, group := range sortedKeys(groupFiles) {
		body := fmt.Sprintf("\nfunc (%s *%s) %s() {\nif %s == nil { panic(\"uninitialized compilation\") }\n", recv, context.Type, group, recv)
		for _, field := range sortedKeys(valueOwners) {
			if fieldGroups[field] == group {
				body += fmt.Sprintf("%s.%s.%s = %s\n", recv, field, valueOwners[field], recv)
			}
		}
		for _, name := range calls {
			if callGroups[name] == group {
				body += recv + "." + name + "()\n"
			}
		}
		helpers[groupFiles[group]] += body + "}\n"
	}
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
			typ := info.TypeOf(n)
			if ptr, ok := typ.(*types.Pointer); ok {
				typ = ptr.Elem()
			}
			found = found || ownedTypeRule(typ, owners, info) != nil
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
func ownedArgumentChecks(fn *ast.FuncDecl, info *types.Info, owners []*OwnerRule, ctx string, nullable []string, preserveNil bool) ([]ast.Stmt, error) {
	var code strings.Builder
	optional := map[string]bool{}
	for _, name := range nullable {
		optional[name] = false
	}
	for _, field := range fn.Type.Params.List {
		t := info.TypeOf(field.Type)
		slice, isSlice := t.(*types.Slice)
		if isSlice {
			t = slice.Elem()
		}
		if ellipsis, ok := field.Type.(*ast.Ellipsis); ok {
			t = info.TypeOf(ellipsis.Elt)
			isSlice = true
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
			if _, ok := optional[name.Name]; ok {
				if isSlice {
					return nil, fmt.Errorf("nullable rule requires a direct owned pointer: %s", name.Name)
				}
				optional[name.Name] = true
			}
			if preserveNil || optional[name.Name] {
				fmt.Fprintf(&code, "if %s != nil {\n", arg)
			}
			fmt.Fprintf(&code, "if %s == nil || %s == nil || %s.%s != %s { panic(\"argument belongs to another or uninitialized compilation\") }\n", ctx, arg, arg, owner.Field, ctx)
			if preserveNil || optional[name.Name] {
				code.WriteString("}\n")
			}
			if isSlice {
				code.WriteString("}\n")
			}
		}
	}
	for name, found := range optional {
		if !found {
			return nil, fmt.Errorf("nullable parameter not found or not owned: %s", name)
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

// Fixed-array len/cap expressions remain constants when their table is moved
// into a Context. Preserve the checked value before resolving runtime state.
func replaceConstantContextLengths(files []*ast.File, info *types.Info, states map[types.Object]string) {
	for _, f := range files {
		replacements := map[ast.Node]ast.Expr{}
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || len(call.Args) != 1 {
				return true
			}
			id, ok := call.Args[0].(*ast.Ident)
			if !ok || states[info.Uses[id]] == "" {
				return true
			}
			fn, ok := call.Fun.(*ast.Ident)
			if !ok {
				return true
			}
			builtin, ok := info.Uses[fn].(*types.Builtin)
			if ok && (builtin.Name() == "len" || builtin.Name() == "cap") && info.Types[call].Value != nil {
				replacements[call] = &ast.BasicLit{Kind: token.INT, Value: info.Types[call].Value.ExactString(), ValuePos: call.Pos()}
			}
			return true
		})
		rewriteExprs(f, replacements)
	}
}

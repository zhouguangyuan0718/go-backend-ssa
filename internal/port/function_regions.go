package port

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"path"
	"reflect"
	"strconv"
	"strings"
)

// A second named region starts from an independent parse of the original
// declaration, before selection or extraction can mutate either interval. This
// retains lexical object identities and comments for boundary validation.
func prepareFunctionRegions(fs *token.FileSet, file *ast.File, data []byte, unit Unit) error {
	original := map[string]*ast.FuncDecl{}
	for _, d := range file.Decls {
		if fn, ok := d.(*ast.FuncDecl); ok && fn.Recv == nil {
			original[fn.Name.Name] = fn
		}
	}
	combined := append([]byte(nil), data...)
	copied := false
	for _, name := range sortedKeys(unit.FunctionRegions) {
		rule := unit.FunctionRegions[name]
		if rule.SourceFunction == "" {
			continue
		}
		if file.Scope.Lookup(name) != nil {
			return fmt.Errorf("function region name collides with native declaration: %s", name)
		}
		source := original[rule.SourceFunction]
		if source == nil || source.Body == nil || source.Type.TypeParams != nil || source.Type.Results != nil {
			return fmt.Errorf("function region source missing or unsupported: %s", rule.SourceFunction)
		}
		start, end := fs.Position(source.Pos()).Offset, fs.Position(source.End()).Offset
		if source.Doc != nil {
			start = fs.Position(source.Doc.Pos()).Offset
		}
		nameStart, nameEnd := fs.Position(source.Name.Pos()).Offset, fs.Position(source.Name.End()).Offset
		combined = append(combined, '\n')
		combined = append(combined, data[start:nameStart]...)
		combined = append(combined, name...)
		combined = append(combined, data[nameEnd:end]...)
		combined = append(combined, '\n')
		copied = true
	}
	if copied {
		// One token file keeps comment lines ordered for go/printer. Parsing
		// the combined source also resolves package identifiers in each copy.
		parsed, e := parser.ParseFile(fs, unit.Source+"#regions", combined, parser.ParseComments)
		if e != nil {
			return e
		}
		*file = *parsed
	}
	return nil
}

// The source AST still carries lexical object identities here. Preserve each
// selected statement verbatim, and make every non-package capture explicit.
func extractFunctionRegions(s *Source, fs *token.FileSet, file *ast.File, unit Unit, importer *packageImporter) (map[string]string, error) {
	hashes := map[string]string{}
	for _, name := range sortedKeys(unit.FunctionRegions) {
		rule := unit.FunctionRegions[name]
		var fn *ast.FuncDecl
		for _, d := range file.Decls {
			if f, ok := d.(*ast.FuncDecl); ok && f.Name.Name == name && f.Recv == nil {
				if fn != nil {
					return nil, fmt.Errorf("ambiguous function region %s", name)
				}
				fn = f
			}
		}
		if fn == nil || fn.Body == nil || fn.Type.TypeParams != nil || fn.Type.Results != nil {
			return nil, fmt.Errorf("unsupported function region %s", name)
		}
		first, last := -1, -1
		for i, stmt := range fn.Body.List {
			matches, e := regionAnchor(file, stmt, rule.First)
			if e != nil {
				return nil, e
			}
			if matches {
				if first >= 0 {
					return nil, fmt.Errorf("ambiguous first region anchor: %s", name)
				}
				first = i
			}
			matches, e = regionAnchor(file, stmt, rule.Last)
			if e != nil {
				return nil, e
			}
			if matches {
				if last >= 0 {
					return nil, fmt.Errorf("ambiguous last region anchor: %s", name)
				}
				last = i
			}
		}
		if rule.EndBefore {
			last--
		}
		if first < 0 || last < first {
			return nil, fmt.Errorf("function region anchors not found in order: %s", name)
		}
		body := fn.Body.List[first : last+1]
		inside := map[ast.Node]bool{}
		for _, stmt := range body {
			ast.Inspect(stmt, func(n ast.Node) bool {
				if n != nil {
					inside[n] = true
				}
				return true
			})
		}
		inputs := map[*ast.Object]string{}
		fields := []*ast.Field{}
		for _, input := range rule.Inputs {
			var obj *ast.Object
			ast.Inspect(fn, func(n ast.Node) bool {
				if id, ok := n.(*ast.Ident); ok && id.Name == input && id.Obj != nil && id.Obj.Kind == ast.Var && !regionDeclarationInside(id.Obj, inside) && file.Scope.Lookup(id.Name) != id.Obj {
					if obj == nil {
						obj = id.Obj
					} else if obj != id.Obj {
						obj = &ast.Object{}
					}
				}
				return true
			})
			if obj == nil || obj.Decl == nil {
				return nil, fmt.Errorf("region input is missing or shadowed: %s", input)
			}
			typ, source, e := regionInputType(fs, file, obj, importer)
			if e != nil {
				return nil, fmt.Errorf("region input %s: %w", input, e)
			}
			if source != "" {
				data, e := s.Read(source)
				if e != nil {
					return nil, e
				}
				hashes[source] = hash(data)
			}
			setTokenPositions(reflect.ValueOf(typ), fn.Type.Params.Opening)
			fields = append(fields, &ast.Field{Names: []*ast.Ident{{Name: input, NamePos: fn.Type.Params.Opening}}, Type: typ})
			inputs[obj] = input
		}
		used := map[*ast.Object]bool{}
		var failure error
		for _, stmt := range body {
			ast.Inspect(stmt, func(n ast.Node) bool {
				if failure != nil {
					return false
				}
				switch n := n.(type) {
				case *ast.FuncLit, *ast.DeferStmt, *ast.GoStmt, *ast.ReturnStmt:
					failure = fmt.Errorf("function region contains a lifetime or return boundary: %s", name)
					return false
				case *ast.BranchStmt:
					if n.Tok == token.GOTO || n.Label != nil {
						failure = fmt.Errorf("function region contains a nonlocal branch: %s", name)
						return false
					}
				case *ast.CallExpr:
					if id, ok := ast.Unparen(n.Fun).(*ast.Ident); ok && id.Name == "recover" && id.Obj == nil {
						failure = fmt.Errorf("function region cannot move recover: %s", name)
						return false
					}
				case *ast.UnaryExpr:
					if n.Op == token.AND && regionInputIdent(n.X, inputs) != nil {
						failure = fmt.Errorf("function region takes input storage address: %s", name)
						return false
					}
				case *ast.AssignStmt:
					for _, lhs := range n.Lhs {
						if regionInputIdent(lhs, inputs) != nil {
							failure = fmt.Errorf("function region writes input storage: %s", name)
							return false
						}
					}
				case *ast.IncDecStmt:
					if regionInputIdent(n.X, inputs) != nil {
						failure = fmt.Errorf("function region changes input storage: %s", name)
						return false
					}
				case *ast.RangeStmt:
					if n.Tok == token.ASSIGN && (regionInputIdent(n.Key, inputs) != nil || regionInputIdent(n.Value, inputs) != nil) {
						failure = fmt.Errorf("function region assigns range input: %s", name)
						return false
					}
				case *ast.Ident:
					obj := n.Obj
					if obj != nil && (obj == fn.Name.Obj || rule.SourceFunction != "" && obj == file.Scope.Lookup(rule.SourceFunction)) {
						failure = fmt.Errorf("function region refers to its unextracted function: %s", name)
						return false
					}
					if obj == nil || file.Scope.Lookup(n.Name) == obj || regionDeclarationInside(obj, inside) {
						return true
					}
					if _, ok := inputs[obj]; !ok {
						failure = fmt.Errorf("function region has an unbound local capture: %s", n.Name)
						return false
					}
					used[obj] = true
				}
				return true
			})
		}
		if failure != nil {
			return nil, failure
		}
		for obj, input := range inputs {
			if !used[obj] {
				return nil, fmt.Errorf("unused function region input: %s", input)
			}
		}
		// A local declared in the extracted region must not feed later source code.
		for i, stmt := range fn.Body.List {
			if i >= first && i <= last {
				continue
			}
			ast.Inspect(stmt, func(n ast.Node) bool {
				if id, ok := n.(*ast.Ident); ok && id.Obj != nil && regionDeclarationInside(id.Obj, inside) {
					failure = fmt.Errorf("function region local escapes its boundary: %s", id.Name)
				}
				return true
			})
		}
		if failure != nil {
			return nil, failure
		}
		oldStart, oldEnd := fn.Body.Pos(), fn.Body.End()
		doc := fn.Doc
		fn.Doc = nil
		fn.Type.Params = &ast.FieldList{Opening: fn.Type.Params.Opening, Closing: fn.Type.Params.Closing, List: fields}
		fn.Body = &ast.BlockStmt{Lbrace: fn.Body.Lbrace, List: body, Rbrace: fn.Body.Rbrace}
		comments := file.Comments[:0]
		for _, c := range file.Comments {
			if c == doc {
				continue
			}
			if c.Pos() >= oldStart && c.End() <= oldEnd && (c.End() < body[0].Pos() || c.Pos() > body[len(body)-1].End()) {
				continue
			}
			comments = append(comments, c)
		}
		file.Comments = comments
	}
	pruneImports(file)
	return hashes, nil
}

// go/parser retains the short-assignment declaration object used while
// parsing a range clause, although its identifiers are moved into RangeStmt.
// Recognize those same declaration identifiers, not unrelated spelling/positions.
func regionDeclarationInside(obj *ast.Object, inside map[ast.Node]bool) bool {
	if obj == nil {
		return false
	}
	decl, _ := obj.Decl.(ast.Node)
	if inside[decl] {
		return true
	}
	if assignment, ok := decl.(*ast.AssignStmt); ok && assignment.Tok == token.DEFINE {
		for _, lhs := range assignment.Lhs {
			if id, ok := lhs.(*ast.Ident); ok && id.Obj == obj && inside[id] {
				return true
			}
		}
	}
	return false
}
func regionInputIdent(expr ast.Expr, inputs map[*ast.Object]string) *ast.Ident {
	if expr == nil {
		return nil
	}
	id, ok := ast.Unparen(expr).(*ast.Ident)
	if ok && inputs[id.Obj] != "" {
		return id
	}
	return nil
}
func regionImports(file *ast.File) map[string]string {
	out := map[string]string{}
	for _, im := range file.Imports {
		p, _ := strconv.Unquote(im.Path.Value)
		name := path.Base(p)
		if im.Name != nil {
			name = im.Name.Name
		}
		out[name] = p
	}
	return out
}
func regionSelector(expr ast.Expr) (*ast.Ident, string) {
	var members []string
	for {
		s, ok := ast.Unparen(expr).(*ast.SelectorExpr)
		if !ok {
			break
		}
		members = append([]string{s.Sel.Name}, members...)
		expr = s.X
	}
	root, _ := ast.Unparen(expr).(*ast.Ident)
	return root, strings.Join(members, ".")
}
func regionAnchor(file *ast.File, stmt ast.Stmt, anchor SourceCallAnchor) (bool, error) {
	if anchor.Assignment != "" {
		a, ok := stmt.(*ast.AssignStmt)
		if !ok || a.Tok != token.ASSIGN || len(a.Lhs) != 1 || len(a.Rhs) != 1 {
			return false, nil
		}
		root, member := regionSelector(a.Lhs[0])
		if root == nil || regionImports(file)[root.Name] != "" || root.Obj != nil && root.Obj != file.Scope.Lookup(root.Name) {
			return false, nil
		}
		name := root.Name
		if member != "" {
			name += "." + member
		}
		return name == anchor.Assignment, nil
	}
	match := func(call *ast.CallExpr) bool {
		root, member := regionSelector(call.Fun)
		if root == nil || root.Obj != nil || member != anchor.Member || regionImports(file)[root.Name] != anchor.Package {
			return false
		}
		if arg := anchor.StringArgument; arg != nil {
			if arg.Index < 0 || arg.Index >= len(call.Args) {
				return false
			}
			lit, ok := ast.Unparen(call.Args[arg.Index]).(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return false
			}
			value, e := strconv.Unquote(lit.Value)
			if e != nil || value != arg.Value {
				return false
			}
		}
		return true
	}
	if anchor.Enclosing == "" {
		e, ok := stmt.(*ast.ExprStmt)
		if !ok {
			return false, nil
		}
		call, ok := ast.Unparen(e.X).(*ast.CallExpr)
		return ok && match(call), nil
	}
	switch anchor.Enclosing {
	case "range":
		if _, ok := stmt.(*ast.RangeStmt); !ok {
			return false, nil
		}
	case "if":
		if _, ok := stmt.(*ast.IfStmt); !ok {
			return false, nil
		}
	default:
		return false, fmt.Errorf("unsupported enclosing region statement: %s", anchor.Enclosing)
	}
	count := 0
	ast.Inspect(stmt, func(n ast.Node) bool {
		if _, ok := n.(*ast.FuncLit); ok {
			return false
		}
		if call, ok := n.(*ast.CallExpr); ok && match(call) {
			count++
		}
		return true
	})
	if count > 1 {
		return false, fmt.Errorf("ambiguous call in enclosing region statement: %s.%s", anchor.Package, anchor.Member)
	}
	return count == 1, nil
}

func regionInputType(fs *token.FileSet, file *ast.File, obj *ast.Object, importer *packageImporter) (ast.Expr, string, error) {
	clone := func(typ ast.Expr) (ast.Expr, string, error) {
		v, e := parser.ParseExpr(nodeText(fs, typ))
		return v, "", e
	}
	switch d := obj.Decl.(type) {
	case *ast.Field:
		return clone(d.Type)
	case *ast.ValueSpec:
		if d.Type != nil {
			return clone(d.Type)
		}
		if len(d.Names) != 1 || len(d.Values) != 1 {
			return nil, "", fmt.Errorf("requires a single typed declaration or initializer")
		}
		return regionInitializerType(fs, file, d.Values[0], importer)
	case *ast.AssignStmt:
		if d.Tok != token.DEFINE || len(d.Lhs) != 1 || len(d.Rhs) != 1 {
			return nil, "", fmt.Errorf("requires a single initializer")
		}
		return regionInitializerType(fs, file, d.Rhs[0], importer)
	}
	return nil, "", fmt.Errorf("unsupported local declaration")
}
func regionInitializerType(fs *token.FileSet, file *ast.File, expr ast.Expr, importer *packageImporter) (ast.Expr, string, error) {
	call, ok := ast.Unparen(expr).(*ast.CallExpr)
	if !ok {
		return nil, "", fmt.Errorf("requires a native factory call")
	}
	root, member := regionSelector(call.Fun)
	imports := regionImports(file)
	if root == nil || root.Obj != nil || member == "" || strings.Contains(member, ".") || imports[root.Name] == "" {
		return nil, "", fmt.Errorf("initializer must name an imported native function")
	}
	pkg, e := importer.Import(imports[root.Name])
	if e != nil {
		return nil, "", e
	}
	fn, ok := pkg.Scope().Lookup(member).(*types.Func)
	if !ok {
		return nil, "", fmt.Errorf("initializer is not a function")
	}
	source := importer.sources[fn]
	if source == "" {
		return nil, "", fmt.Errorf("initializer signature has no pinned source")
	}
	sig := fn.Type().(*types.Signature)
	if sig.TypeParams().Len() != 0 || sig.Results().Len() != 1 {
		return nil, "", fmt.Errorf("initializer requires a non-generic single-result signature")
	}
	var missing bool
	text := types.TypeString(sig.Results().At(0).Type(), func(p *types.Package) string {
		for alias, path := range imports {
			if path == p.Path() && alias != "." && alias != "_" {
				return alias
			}
		}
		missing = true
		return p.Name()
	})
	if missing {
		return nil, "", fmt.Errorf("initializer result requires an unbound import")
	}
	typ, e := parser.ParseExpr(text)
	return typ, source, e
}
func checkFunctionRegionInputs(files []*ast.File, info *types.Info, units []Unit) error {
	for i, u := range units {
		for _, d := range files[i].Decls {
			fn, ok := d.(*ast.FuncDecl)
			if !ok {
				continue
			}
			if _, ok := u.FunctionRegions[fn.Name.Name]; !ok {
				continue
			}
			for _, f := range fn.Type.Params.List {
				for _, id := range f.Names {
					obj := info.Defs[id]
					if obj == nil {
						return fmt.Errorf("function region input type missing: %s", id.Name)
					}
					if _, ok := types.Unalias(obj.Type()).(*types.Pointer); !ok {
						return fmt.Errorf("function region requires pointer input: %s", id.Name)
					}
				}
			}
		}
	}
	return nil
}

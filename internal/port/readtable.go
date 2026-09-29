package port

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"reflect"
)

// Offset and name tables contain constant array elements. Localize the table
// once per consuming function, retaining the upstream array type and indexing.
// Only len(table) and indexed reads are accepted, never writes or escapes.
func localizeReadTables(fs *token.FileSet, files []*ast.File, info *types.Info, units []Unit) error {
	tables := map[types.Object]*ast.ValueSpec{}
	for i, u := range units {
		for _, name := range u.ReadTables {
			found := false
			for _, d := range files[i].Decls {
				g, ok := d.(*ast.GenDecl)
				if !ok || g.Tok != token.VAR {
					continue
				}
				for _, spec := range g.Specs {
					v := spec.(*ast.ValueSpec)
					if len(v.Names) != 1 || v.Names[0].Name != name {
						continue
					}
					found = true
					if len(v.Values) != 1 {
						return fmt.Errorf("read table requires constant array initializer: %s", name)
					}
					literal, ok := v.Values[0].(*ast.CompositeLit)
					if !ok {
						return fmt.Errorf("read table requires array literal")
					}
					array, ok := info.TypeOf(literal).(*types.Array)
					if !ok {
						return fmt.Errorf("read table requires array type")
					}
					basic, ok := array.Elem().Underlying().(*types.Basic)
					if !ok || basic.Info()&(types.IsInteger|types.IsString) == 0 {
						return fmt.Errorf("read table requires integer or string elements")
					}
					for _, expr := range literal.Elts {
						if pair, ok := expr.(*ast.KeyValueExpr); ok {
							if info.Types[pair.Key].Value == nil {
								return fmt.Errorf("nonconstant read table key")
							}
							expr = pair.Value
						}
						if info.Types[expr].Value == nil {
							return fmt.Errorf("nonconstant read table element")
						}
					}
					tables[info.Defs[v.Names[0]]] = v
				}
			}
			if !found {
				return fmt.Errorf("read table not found: %s", name)
			}
		}
	}
	if len(tables) == 0 {
		return nil
	}
	for _, f := range files {
		parents := map[ast.Node]ast.Node{}
		var stack []ast.Node
		ast.Inspect(f, func(n ast.Node) bool {
			if n == nil {
				stack = stack[:len(stack)-1]
				return true
			}
			if len(stack) > 0 {
				parents[n] = stack[len(stack)-1]
			}
			stack = append(stack, n)
			return true
		})
		useByFunc := map[*ast.FuncDecl]map[types.Object]bool{}
		for id, obj := range info.Uses {
			if tables[obj] == nil || parents[id] == nil {
				continue
			}
			parent := parents[id]
			read := false
			switch p := parent.(type) {
			case *ast.IndexExpr:
				if p.X == id {
					read = true
					top := ast.Node(p)
					for {
						if paren, ok := parents[top].(*ast.ParenExpr); ok {
							top = paren
							continue
						}
						break
					}
					switch outer := parents[top].(type) {
					case *ast.AssignStmt:
						for _, lhs := range outer.Lhs {
							if lhs == top {
								read = false
							}
						}
					case *ast.IncDecStmt:
						read = false
					case *ast.UnaryExpr:
						if outer.Op == token.AND {
							read = false
						}
					}
				}
			case *ast.CallExpr:
				if name, ok := p.Fun.(*ast.Ident); ok && len(p.Args) == 1 && p.Args[0] == id {
					if b, ok := info.Uses[name].(*types.Builtin); ok && b.Name() == "len" {
						read = true
					}
				}
			}
			if !read {
				return fmt.Errorf("read table %s has a write or unsupported escape", obj.Name())
			}
			var fn *ast.FuncDecl
			for p := parent; p != nil; p = parents[p] {
				if candidate, ok := p.(*ast.FuncDecl); ok {
					fn = candidate
					break
				}
			}
			if fn == nil {
				return fmt.Errorf("read table used outside a function")
			}
			if useByFunc[fn] == nil {
				useByFunc[fn] = map[types.Object]bool{}
			}
			useByFunc[fn][obj] = true
		}
		// Follow declaration order, never map iteration order, for stable output.
		for _, d := range f.Decls {
			fn, ok := d.(*ast.FuncDecl)
			if !ok {
				continue
			}
			names := map[string]types.Object{}
			for obj := range useByFunc[fn] {
				names[obj.Name()] = obj
			}
			var statements []ast.Stmt
			for _, name := range sortedKeys(names) {
				v := tables[names[name]]
				parsed, err := parser.ParseFile(token.NewFileSet(), "table.go", "package p;func f(){var "+nodeText(fs, v)+"}", parser.SkipObjectResolution)
				if err != nil {
					return err
				}
				setTokenPositions(reflect.ValueOf(parsed), fn.Body.Lbrace)
				statements = append(statements, parsed.Decls[0].(*ast.FuncDecl).Body.List...)
			}
			fn.Body.List = append(statements, fn.Body.List...)
		}
		comments := ast.NewCommentMap(fs, f, f.Comments)
		var decls []ast.Decl
		for _, d := range f.Decls {
			if g, ok := d.(*ast.GenDecl); ok && g.Tok == token.VAR {
				var specs []ast.Spec
				for _, spec := range g.Specs {
					v := spec.(*ast.ValueSpec)
					remove := false
					for _, id := range v.Names {
						remove = remove || tables[info.Defs[id]] != nil
					}
					if !remove {
						specs = append(specs, spec)
					}
				}
				g.Specs = specs
				if len(specs) == 0 {
					continue
				}
			}
			decls = append(decls, d)
		}
		f.Decls = decls
		f.Comments = comments.Filter(f).Comments()
	}
	var initializers []*types.Initializer
	for _, init := range info.InitOrder {
		if len(init.Lhs) != 1 || tables[init.Lhs[0]] == nil {
			initializers = append(initializers, init)
		}
	}
	info.InitOrder = initializers
	return nil
}

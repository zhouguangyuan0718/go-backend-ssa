package port

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"reflect"
	"strings"
)

// Move small package state into an existing upstream receiver. Public functions,
// receiver types and algorithms keep their signatures. Explicit init methods
// reset state on reuse; other receivers initialize at their native allocations.
func moveReceiverState(fs *token.FileSet, files []*ast.File, info *types.Info, units []Unit) error {
	for index, u := range units {
		rule := u.ReceiverState
		if rule == nil {
			continue
		}
		states := map[types.Object]string{}
		specs := map[string]*ast.ValueSpec{}
		var target *ast.StructType
		var targetType types.Type
		for _, d := range files[index].Decls {
			g, ok := d.(*ast.GenDecl)
			if !ok {
				continue
			}
			for _, spec := range g.Specs {
				switch s := spec.(type) {
				case *ast.TypeSpec:
					if s.Name.Name == rule.Type {
						target, _ = s.Type.(*ast.StructType)
						targetType = info.Defs[s.Name].Type()
					}
				case *ast.ValueSpec:
					if g.Tok != token.VAR {
						continue
					}
					for _, id := range s.Names {
						if field := rule.Fields[id.Name]; field != "" {
							if len(s.Names) != 1 || len(s.Values) > 1 {
								return fmt.Errorf("receiver state requires single declarations: %s", id.Name)
							}
							states[info.Defs[id]] = field
							specs[id.Name] = s
						}
					}
				}
			}
		}
		if target == nil {
			return fmt.Errorf("receiver state target is not a selected struct: %s", rule.Type)
		}
		// Parse cloned expressions so generated occurrences do not share AST nodes or
		// drag the original declaration's comments into a method body.
		clone := func(expr ast.Expr, pos token.Pos) (ast.Expr, error) {
			e, err := parser.ParseExpr(nodeText(fs, expr))
			if err == nil {
				setTokenPositions(reflect.ValueOf(e), pos)
			}
			return e, err
		}
		fieldTypes := map[string]ast.Expr{}
		for _, name := range sortedKeys(rule.Fields) {
			spec := specs[name]
			if spec == nil {
				return fmt.Errorf("receiver state declaration not found: %s", name)
			}
			field := rule.Fields[name]
			for _, f := range target.Fields.List {
				for _, id := range f.Names {
					if id.Name == field {
						return fmt.Errorf("receiver field collision: %s", field)
					}
				}
			}
			typ := spec.Type
			if typ == nil {
				// Inferred state is deliberately restricted to fresh struct pointers and
				// literal byte slices, the reviewed syntax branch/printer initializers.
				call, ok := spec.Values[0].(*ast.CallExpr)
				if !ok || len(call.Args) != 1 {
					return fmt.Errorf("unsupported inferred receiver state: %s", name)
				}
				switch fun := call.Fun.(type) {
				case *ast.Ident:
					b, ok := info.Uses[fun].(*types.Builtin)
					if !ok || b.Name() != "new" {
						return fmt.Errorf("receiver initializer must use builtin new: %s", name)
					}
					if _, ok := info.TypeOf(call.Args[0]).Underlying().(*types.Struct); !ok {
						return fmt.Errorf("receiver new requires struct: %s", name)
					}
					typ = &ast.StarExpr{X: call.Args[0]}
				case *ast.ArrayType:
					slice, ok := info.TypeOf(fun).(*types.Slice)
					if !ok || !types.Identical(slice.Elem(), types.Typ[types.Byte]) || info.Types[call.Args[0]].Value == nil {
						return fmt.Errorf("receiver initializer requires constant byte slice: %s", name)
					}
					typ = fun
				default:
					return fmt.Errorf("unsupported receiver initializer: %s", name)
				}
			}
			fieldTypes[name] = typ
			copy, err := clone(typ, target.Fields.Closing)
			if err != nil {
				return err
			}
			target.Fields.List = append(target.Fields.List, &ast.Field{Names: []*ast.Ident{{Name: field, NamePos: target.Fields.Closing}}, Type: copy})
		}
		// len/cap of a fixed array is a compile-time value, not a read of its
		// contents. Preserve that value before checking for state in free helpers.
		for _, f := range files {
			replacements := map[ast.Node]ast.Expr{}
			ast.Inspect(f, func(n ast.Node) bool {
				c, ok := n.(*ast.CallExpr)
				if !ok || len(c.Args) != 1 {
					return true
				}
				id, ok := c.Args[0].(*ast.Ident)
				if !ok || states[info.Uses[id]] == "" {
					return true
				}
				fn, ok := c.Fun.(*ast.Ident)
				if !ok {
					return true
				}
				b, ok := info.Uses[fn].(*types.Builtin)
				if ok && (b.Name() == "len" || b.Name() == "cap") && info.Types[c].Value != nil {
					replacements[c] = &ast.BasicLit{Kind: token.INT, Value: info.Types[c].Value.ExactString(), ValuePos: c.Pos()}
				}
				return true
			})
			rewriteExprs(f, replacements)
		}
		initCount := 0
		var initCalls []string
		var start *ast.FuncDecl
		for fileIndex, f := range files {
			for _, d := range f.Decls {
				fn, ok := d.(*ast.FuncDecl)
				if !ok {
					continue
				}
				if fn.Recv == nil && fn.Name.Name == "init" && fileIndex == index {
					if !rule.RunInit || rule.InitMethod == "" {
						return fmt.Errorf("receiver package init requires explicit rule")
					}
					initCount++
					name := fmt.Sprintf("syncgoInitState%d", initCount)
					fn.Name = &ast.Ident{Name: name, NamePos: fn.Name.Pos()}
					fn.Recv = &ast.FieldList{List: []*ast.Field{{Names: []*ast.Ident{{Name: "syncgoState", NamePos: fn.Type.Func}}, Type: &ast.StarExpr{Star: fn.Type.Func, X: &ast.Ident{Name: rule.Type, NamePos: fn.Type.Func}}}}}
					initCalls = append(initCalls, name)
				}
				receiver := ""
				if receiverType(fn) == rule.Type && len(fn.Recv.List[0].Names) == 1 {
					receiver = fn.Recv.List[0].Names[0].Name
					if fn.Name.Name == rule.InitMethod {
						start = fn
					}
				}
				replacements := map[*ast.Ident]ast.Expr{}
				bad := ""
				ast.Inspect(fn.Body, func(n ast.Node) bool {
					if id, ok := n.(*ast.Ident); ok {
						if field := states[info.Uses[id]]; field != "" {
							if receiver == "" {
								bad = id.Name
							} else {
								replacements[id] = &ast.SelectorExpr{X: &ast.Ident{Name: receiver, NamePos: id.Pos()}, Sel: &ast.Ident{Name: field, NamePos: id.Pos()}}
							}
						}
					}
					return true
				})
				if bad != "" {
					return fmt.Errorf("receiver state %s used outside %s methods", bad, rule.Type)
				}
				rewriteNodes(fn.Body, replacements)
			}
		}
		if rule.RunInit && initCount == 0 {
			return fmt.Errorf("receiver init declaration missing")
		}
		if rule.InitMethod != "" {
			if start == nil {
				return fmt.Errorf("receiver initialization method missing: %s", rule.InitMethod)
			}
			recv := start.Recv.List[0].Names[0].Name
			var code strings.Builder
			for _, name := range sortedKeys(specs) {
				spec := specs[name]
				if len(spec.Values) > 0 {
					fmt.Fprintf(&code, "%s.%s = %s;", recv, rule.Fields[name], nodeText(fs, spec.Values[0]))
				} else {
					fmt.Fprintf(&code, "{var syncgoZero %s; %s.%s = syncgoZero};", nodeText(fs, fieldTypes[name]), recv, rule.Fields[name])
				}
			}
			for _, name := range initCalls {
				fmt.Fprintf(&code, "%s.%s();", recv, name)
			}
			statements, err := contextStatements(code.String(), start.Body.Lbrace)
			if err != nil {
				return err
			}
			start.Body.List = append(statements, start.Body.List...)
		} else {
			// All valid construction sites must use a keyed literal or builtin new;
			// a bare zero declaration would silently lose the relocated initializer.
			for _, f := range files {
				replacements := map[ast.Node]ast.Expr{}
				var failure error
				fields := func(pos token.Pos) []ast.Expr {
					var out []ast.Expr
					for _, name := range sortedKeys(specs) {
						spec := specs[name]
						if len(spec.Values) == 0 {
							continue
						}
						value, err := clone(spec.Values[0], pos)
						if err != nil {
							failure = err
							return nil
						}
						out = append(out, &ast.KeyValueExpr{Key: &ast.Ident{Name: rule.Fields[name], NamePos: pos}, Value: value})
					}
					return out
				}
				ast.Inspect(f, func(n ast.Node) bool {
					switch n := n.(type) {
					case *ast.CompositeLit:
						if info.TypeOf(n.Type) == targetType {
							for _, e := range n.Elts {
								if _, ok := e.(*ast.KeyValueExpr); !ok {
									failure = fmt.Errorf("unkeyed receiver state allocation: %s", rule.Type)
									return false
								}
							}
							n.Elts = append(n.Elts, fields(n.Lbrace)...)
						}
					case *ast.ValueSpec:
						if n.Type != nil && len(n.Values) == 0 && info.TypeOf(n.Type) == targetType {
							failure = fmt.Errorf("zero receiver state declaration: %s", rule.Type)
						}
					case *ast.CallExpr:
						if id, ok := n.Fun.(*ast.Ident); ok && len(n.Args) == 1 && info.TypeOf(n.Args[0]) == targetType {
							if b, ok := info.Uses[id].(*types.Builtin); ok && b.Name() == "new" {
								replacements[n] = &ast.UnaryExpr{Op: token.AND, X: &ast.CompositeLit{Type: n.Args[0], Elts: fields(n.Pos())}}
							}
						}
					}
					return true
				})
				if failure != nil {
					return failure
				}
				rewriteExprs(f, replacements)
			}
		}
		comments := ast.NewCommentMap(fs, files[index], files[index].Comments)
		var decls []ast.Decl
		for _, d := range files[index].Decls {
			if g, ok := d.(*ast.GenDecl); ok && g.Tok == token.VAR {
				var keep []ast.Spec
				for _, sp := range g.Specs {
					v := sp.(*ast.ValueSpec)
					if specs[v.Names[0].Name] == nil {
						keep = append(keep, sp)
					}
				}
				g.Specs = keep
				if len(keep) == 0 {
					continue
				}
			}
			decls = append(decls, d)
		}
		files[index].Decls = decls
		files[index].Comments = comments.Filter(files[index]).Comments()
		var pending []*types.Initializer
		for _, entry := range info.InitOrder {
			if len(entry.Lhs) != 1 || states[entry.Lhs[0]] == "" {
				pending = append(pending, entry)
			}
		}
		info.InitOrder = pending
	}
	return nil
}

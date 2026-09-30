package port

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"reflect"
)

type functionContinuation struct {
	fn     *ast.FuncDecl
	first  ast.Stmt
	state  string
	method string
}

// Resolve anchors and lexical state while the input still has native identities.
// Branches cannot cross the new closure boundary; named result variables and
// generic functions need separate contracts and are intentionally unsupported.
func prepareFunctionContinuations(files []*ast.File, info *types.Info, units []Unit) ([]functionContinuation, error) {
	var result []functionContinuation
	for i, file := range files {
		for _, name := range sortedKeys(units[i].FunctionContinuations) {
			rule := units[i].FunctionContinuations[name]
			var fn *ast.FuncDecl
			for _, decl := range file.Decls {
				if f, ok := decl.(*ast.FuncDecl); ok && f.Recv == nil && f.Name.Name == name {
					if fn != nil {
						return nil, fmt.Errorf("ambiguous continuation function %s", name)
					}
					fn = f
				}
			}
			if fn == nil || fn.Body == nil || fn.Type.TypeParams != nil || fn.Type.Results == nil || len(fn.Type.Results.List) != 1 || len(fn.Type.Results.List[0].Names) != 0 {
				return nil, fmt.Errorf("continuation requires one unnamed pointer result: %s", name)
			}
			if _, ok := fn.Type.Results.List[0].Type.(*ast.StarExpr); !ok {
				return nil, fmt.Errorf("continuation requires pointer result: %s", name)
			}
			first := -1
			for j, stmt := range fn.Body.List {
				match, err := regionAnchor(file, stmt, rule.Before)
				if err != nil {
					return nil, err
				}
				if match {
					if first >= 0 {
						return nil, fmt.Errorf("ambiguous continuation boundary: %s", name)
					}
					first = j
				}
			}
			if first <= 0 || first >= len(fn.Body.List)-1 {
				return nil, fmt.Errorf("continuation boundary missing or empty: %s", name)
			}
			if _, ok := fn.Body.List[len(fn.Body.List)-1].(*ast.ReturnStmt); !ok {
				return nil, fmt.Errorf("continuation requires a final return: %s", name)
			}
			var state types.Object
			var invalid bool
			ast.Inspect(fn.Body, func(node ast.Node) bool {
				switch n := node.(type) {
				case *ast.FuncLit:
					return false
				case *ast.BranchStmt:
					if n.Tok == token.GOTO {
						invalid = true
					}
				case *ast.LabeledStmt:
					invalid = true
				case *ast.Ident:
					if n.Name == rule.State && info.Defs[n] != nil {
						if state != nil || n.Pos() >= fn.Body.List[first].Pos() {
							invalid = true
						}
						state = info.Defs[n]
					}
				}
				return true
			})
			if invalid || state == nil {
				return nil, fmt.Errorf("continuation state missing, shadowed or crossing a branch: %s", name)
			}
			if _, ok := state.(*types.Var); !ok {
				return nil, fmt.Errorf("continuation state is not a local variable: %s", name)
			}
			if _, ok := state.Type().Underlying().(*types.Struct); !ok {
				return nil, fmt.Errorf("continuation state requires a struct: %s", name)
			}
			// The state must be declared in the function's top-level prefix, so its
			// address is valid at the suffix boundary, not inside an earlier branch.
			top := false
			for _, stmt := range fn.Body.List[:first] {
				switch stmt := stmt.(type) {
				case *ast.AssignStmt:
					for _, lhs := range stmt.Lhs {
						if id, ok := lhs.(*ast.Ident); ok && info.Defs[id] == state {
							top = true
						}
					}
				case *ast.DeclStmt:
					if decl, ok := stmt.Decl.(*ast.GenDecl); ok {
						for _, spec := range decl.Specs {
							if vs, ok := spec.(*ast.ValueSpec); ok {
								for _, id := range vs.Names {
									if info.Defs[id] == state {
										top = true
									}
								}
							}
						}
					}
				}
			}
			if !top {
				return nil, fmt.Errorf("continuation state is not a top-level local: %s", name)
			}
			result = append(result, functionContinuation{fn, fn.Body.List[first], rule.State, rule.Method})
		}
	}
	return result, nil
}

func applyFunctionContinuations(fs *token.FileSet, bindings []functionContinuation) error {
	for _, binding := range bindings {
		fn := binding.fn
		if fn.Recv == nil || len(fn.Recv.List) != 1 || len(fn.Recv.List[0].Names) != 1 {
			return fmt.Errorf("continuation requires an instance receiver: %s", fn.Name.Name)
		}
		first := -1
		for i, stmt := range fn.Body.List {
			if stmt == binding.first {
				first = i
				break
			}
		}
		if first < 0 {
			return fmt.Errorf("continuation boundary changed: %s", fn.Name.Name)
		}
		pos := binding.first.Pos()
		resultType, err := parser.ParseExpr(nodeText(fs, fn.Type.Results.List[0].Type))
		if err != nil {
			return err
		}
		setTokenPositions(reflect.ValueOf(resultType), pos)
		tail := &ast.FuncLit{Type: &ast.FuncType{Func: pos, Params: &ast.FieldList{Opening: pos, Closing: pos}, Results: &ast.FieldList{List: []*ast.Field{{Type: resultType}}}}, Body: &ast.BlockStmt{Lbrace: pos, List: append([]ast.Stmt(nil), fn.Body.List[first:]...), Rbrace: fn.Body.Rbrace}}
		call := &ast.CallExpr{Fun: &ast.SelectorExpr{X: ast.NewIdent(fn.Recv.List[0].Names[0].Name), Sel: ast.NewIdent(binding.method)}, Args: []ast.Expr{&ast.UnaryExpr{Op: token.AND, X: ast.NewIdent(binding.state)}, tail}}
		fn.Body.List = append(fn.Body.List[:first], &ast.ReturnStmt{Return: pos, Results: []ast.Expr{call}})
	}
	return nil
}

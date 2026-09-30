package port

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
)

// Only identity/nil comparisons of pointer parameters with constant returns can
// precede owner validation. This admits no calls, reads through pointers,
// package state, allocation or mutation before the ordinary owner checks.
func checkContextGuardPrefix(fn *ast.FuncDecl, info *types.Info, count int) error {
	fail := func() error { return fmt.Errorf("context guard prefix is not pure pointer guards: %s", fn.Name.Name) }
	if fn.Body == nil || count > len(fn.Body.List) {
		return fail()
	}
	parameters := map[types.Object]bool{}
	for _, list := range []*ast.FieldList{fn.Recv, fn.Type.Params} {
		if list != nil {
			for _, field := range list.List {
				for _, id := range field.Names {
					if _, ok := info.TypeOf(id).(*types.Pointer); ok {
						parameters[info.Defs[id]] = true
					}
				}
			}
		}
	}
	operand := func(expr ast.Expr) bool {
		id, ok := ast.Unparen(expr).(*ast.Ident)
		if !ok {
			return false
		}
		object := info.Uses[id]
		return parameters[object] || object == types.Universe.Lookup("nil")
	}
	for _, stmt := range fn.Body.List[:count] {
		branch, ok := stmt.(*ast.IfStmt)
		if !ok || branch.Init != nil || branch.Else != nil || len(branch.Body.List) != 1 {
			return fail()
		}
		condition, ok := ast.Unparen(branch.Cond).(*ast.BinaryExpr)
		if !ok || (condition.Op != token.EQL && condition.Op != token.NEQ) || !operand(condition.X) || !operand(condition.Y) {
			return fail()
		}
		result, ok := branch.Body.List[0].(*ast.ReturnStmt)
		if !ok || len(result.Results) == 0 {
			return fail()
		}
		for _, expr := range result.Results {
			if info.Types[expr].Value == nil {
				return fail()
			}
		}
	}
	return nil
}

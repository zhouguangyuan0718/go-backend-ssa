package port

import (
	"go/ast"
	"go/constant"
	"go/types"
)

// Like Caller/Callers, the native panic diagnostic observes only this goroutine.
// Do not extend that permission to the all-goroutine operation or function values.
func currentStackOperations(files []*ast.File, info *types.Info) map[ast.Node]bool {
	approved := map[ast.Node]bool{}
	for _, file := range files {
		ast.Inspect(file, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok || len(call.Args) != 2 || call.Ellipsis.IsValid() {
				return true
			}
			selector, ok := ast.Unparen(call.Fun).(*ast.SelectorExpr)
			if !ok {
				return true
			}
			fn, ok := info.Uses[selector.Sel].(*types.Func)
			if !ok || fn.Pkg() == nil || fn.Pkg().Path() != "runtime" || fn.Name() != "Stack" || fn.Parent() != fn.Pkg().Scope() {
				return true
			}
			value := info.Types[call.Args[1]].Value
			if value != nil && value.Kind() == constant.Bool && !constant.BoolVal(value) {
				approved[selector.Sel] = true
			}
			return true
		})
	}
	return approved
}

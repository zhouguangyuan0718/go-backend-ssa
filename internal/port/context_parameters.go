package port

import (
	"fmt"
	"go/ast"
	"go/types"
)

// Private slice receivers and nullable helper receivers cannot carry an owner.
// Preserve the receiver and thread an explicit context through reviewed methods.
func contextParameterMethods(files []*ast.File, info *types.Info, units []Unit) (map[types.Object]bool, error) {
	methods := map[types.Object]bool{}
	for i, u := range units {
		for _, name := range u.ContextParams {
			found := false
			for _, d := range files[i].Decls {
				fn, ok := d.(*ast.FuncDecl)
				if !ok || fn.Recv == nil || receiverType(fn)+"."+fn.Name.Name != name {
					continue
				}
				methods[info.Defs[fn.Name]] = true
				found = true
			}
			if !found {
				return nil, fmt.Errorf("context parameter method not selected: %s", name)
			}
		}
	}
	return methods, nil
}

func threadMethodContext(node ast.Node, info *types.Info, methods map[types.Object]bool, ctx ast.Expr) error {
	// First classify all uses. A method value has a different function type after
	// adding a parameter; reject it instead of silently changing a callback API.
	calls := map[*ast.SelectorExpr]*ast.CallExpr{}
	ast.Inspect(node, func(n ast.Node) bool {
		if call, ok := n.(*ast.CallExpr); ok {
			fun := call.Fun
			for {
				paren, ok := fun.(*ast.ParenExpr)
				if !ok {
					break
				}
				fun = paren.X
			}
			if sel, ok := fun.(*ast.SelectorExpr); ok {
				calls[sel] = call
			}
		}
		return true
	})
	var err error
	ast.Inspect(node, func(n ast.Node) bool {
		sel, ok := n.(*ast.SelectorExpr)
		if !ok || !methods[info.Uses[sel.Sel]] {
			return true
		}
		call := calls[sel]
		selection := info.Selections[sel]
		if call == nil || selection == nil {
			err = fmt.Errorf("context parameter method value requires explicit adaptation: %s", sel.Sel.Name)
			return false
		}
		index := 0
		if selection.Kind() == types.MethodExpr {
			index = 1
		}
		if len(call.Args) < index {
			err = fmt.Errorf("context parameter method expression lacks receiver")
			return false
		}
		arg := positionedContext(ctx, sel.Pos())
		args := append([]ast.Expr(nil), call.Args[:index]...)
		args = append(args, arg)
		call.Args = append(args, call.Args[index:]...)
		return true
	})
	return err
}

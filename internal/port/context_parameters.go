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

func threadMethodContext(fs *token.FileSet, files []*ast.File, node ast.Node, info *types.Info, methods map[types.Object]bool, ctx ast.Expr, contextType string) error {
	// Direct calls receive the extra argument. Method values use a typed
	// closure retaining the original callback signature and receiver binding.
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
	values := map[ast.Node]ast.Expr{}
	ast.Inspect(node, func(n ast.Node) bool {
		if err != nil {
			return false
		}
		sel, ok := n.(*ast.SelectorExpr)
		if !ok || !methods[info.Uses[sel.Sel]] {
			return true
		}
		call := calls[sel]
		selection := info.Selections[sel]
		if selection == nil {
			err = fmt.Errorf("context parameter method has no selection: %s", sel.Sel.Name)
			return false
		}
		if call == nil {
			receiver := &ast.ParenExpr{X: sel.X}
			if err = threadMethodContext(fs, files, receiver, info, methods, ctx, contextType); err != nil {
				return false
			}
			sel.X = receiver.X
			if methodRecovers(files, info, selection.Obj()) {
				err = fmt.Errorf("context method value with direct recover requires explicit adaptation: %s", sel.Sel.Name)
				return false
			}
			var file *ast.File
			for _, f := range files {
				if sel.Pos() >= f.Pos() && sel.End() <= f.End() {
					file = f
					break
				}
			}
			if file == nil {
				err = fmt.Errorf("context parameter method value has no source file")
				return false
			}
			var bound ast.Expr
			bound, err = bindContextMethod(fs, file, sel, selection, info, ctx, contextType)
			if err == nil {
				values[sel] = bound
			}
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
	if err == nil {
		rewriteExprs(node, values)
	}
	return err
}

// Capturing a receiver must evaluate it exactly once when the method value is
// created, including the original implicit address/dereference operation.
func bindContextMethod(fs *token.FileSet, file *ast.File, sel *ast.SelectorExpr, selection *types.Selection, info *types.Info, ctx ast.Expr, contextType string) (ast.Expr, error) {
	if len(selection.Index()) != 1 {
		return nil, fmt.Errorf("promoted context method value requires explicit adaptation: %s", sel.Sel.Name)
	}
	sig := selection.Type().(*types.Signature)
	qualifier := typeQualifier(file, info, selection.Obj().Pkg())
	var params, results, args []string
	for i := 0; i < sig.Params().Len(); i++ {
		typ := sig.Params().At(i).Type()
		prefix := ""
		if sig.Variadic() && i == sig.Params().Len()-1 {
			typ = typ.(*types.Slice).Elem()
			prefix = "..."
		}
		name := fmt.Sprintf("syncgoArg%d", i)
		params = append(params, name+" "+prefix+types.TypeString(typ, qualifier))
		args = append(args, name)
	}
	if sig.Variadic() {
		args[len(args)-1] += "..."
	}
	for i := 0; i < sig.Results().Len(); i++ {
		results = append(results, types.TypeString(sig.Results().At(i).Type(), qualifier))
	}
	signature := "func(" + strings.Join(params, ",") + ")"
	if len(results) > 0 {
		signature += " (" + strings.Join(results, ",") + ")"
	}
	var call, prefix, suffix string
	if selection.Kind() == types.MethodExpr {
		callArgs := append([]string{args[0], "syncgoContext"}, args[1:]...)
		call = nodeText(fs, sel) + "(" + strings.Join(callArgs, ",") + ")"
		prefix = "func(syncgoContext *" + contextType + ") " + signature + " {return "
		suffix = "}(" + nodeText(fs, ctx) + ")"
	} else {
		declared := selection.Obj().Type().(*types.Signature).Recv().Type()
		receiver := nodeText(fs, sel.X)
		actual := selection.Recv()
		if !types.Identical(declared, actual) {
			if p, ok := declared.(*types.Pointer); ok && types.Identical(p.Elem(), actual) {
				receiver = "&(" + receiver + ")"
			} else if p, ok := actual.(*types.Pointer); ok && types.Identical(p.Elem(), declared) {
				receiver = "*(" + receiver + ")"
			} else {
				return nil, fmt.Errorf("unsupported context method receiver binding: %s", sel.Sel.Name)
			}
		}
		callArgs := append([]string{"syncgoContext"}, args...)
		call = "syncgoReceiver." + sel.Sel.Name + "(" + strings.Join(callArgs, ",") + ")"
		prefix = "func(syncgoReceiver " + types.TypeString(declared, qualifier) + ",syncgoContext *" + contextType + ") " + signature + " {return "
		suffix = "}(" + receiver + "," + nodeText(fs, ctx) + ")"
	}
	if len(results) > 0 {
		call = "return " + call
	}
	code := prefix + signature + " {" + call + "}" + suffix
	bound, err := parser.ParseExpr(code)
	if err != nil {
		return nil, err
	}
	setTokenPositions(reflect.ValueOf(bound), sel.Pos())
	return bound, nil
}

func methodRecovers(files []*ast.File, info *types.Info, method types.Object) bool {
	for _, file := range files {
		for _, d := range file.Decls {
			fn, ok := d.(*ast.FuncDecl)
			if !ok || info.Defs[fn.Name] != method {
				continue
			}
			found := false
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				if _, ok := n.(*ast.FuncLit); ok {
					return false
				}
				if id, ok := n.(*ast.Ident); ok {
					if b, ok := info.Uses[id].(*types.Builtin); ok && b.Name() == "recover" {
						found = true
					}
				}
				return true
			})
			return found
		}
	}
	return false
}

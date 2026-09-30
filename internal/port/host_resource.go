package port

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
)

func hostResourceFactory(obj types.Object, rule HostResourceRule) bool {
	fn, ok := obj.(*types.Func)
	return ok && fn.Pkg() != nil && fn.Pkg().Path() == rule.Package && fn.Name() == rule.Factory && fn.Parent() == fn.Pkg().Scope()
}

func reviewedHostResources(files []*ast.File, before, after *types.Info, pkg *types.Package, units []Unit) (map[ast.Node]bool, error) {
	active := false
	for _, unit := range units {
		active = active || len(unit.HostResources) != 0
	}
	if !active {
		return nil, nil
	}
	reviewed := map[ast.Node]bool{}
	parents := map[ast.Node]ast.Node{}
	for _, f := range files {
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
	}
	for _, unit := range units {
		for _, rule := range unit.HostResources {
			var original *types.Func
			for _, obj := range before.Uses {
				if hostResourceFactory(obj, rule) {
					original = obj.(*types.Func)
					break
				}
			}
			if original == nil {
				return nil, fmt.Errorf("host resource factory not used: %s.%s", rule.Package, rule.Factory)
			}
			context := pkg.Scope().Lookup(unit.Context.Type)
			if context == nil {
				return nil, fmt.Errorf("host resource context missing")
			}
			object, _, _ := types.LookupFieldOrMethod(types.NewPointer(context.Type()), true, pkg, rule.Method)
			method, ok := object.(*types.Func)
			if !ok {
				return nil, fmt.Errorf("host resource adapter method missing: %s", rule.Method)
			}
			signature := method.Type().(*types.Signature)
			want := types.TypeString(unnamedSignature(original.Type()), nil)
			if got := types.TypeString(unnamedSignature(signature), nil); got != want {
				return nil, fmt.Errorf("host resource adapter signature mismatch: %s: got %s, want %s", rule.Method, got, want)
			}
			if signature.Results().Len() != 1 {
				return nil, fmt.Errorf("host resource requires one result")
			}
			pointer, ok := signature.Results().At(0).Type().(*types.Pointer)
			if !ok {
				return nil, fmt.Errorf("host resource requires pointer result")
			}
			named, ok := pointer.Elem().(*types.Named)
			if !ok || named.Obj().Pkg() == nil || named.Obj().Pkg().Path() != rule.Package {
				return nil, fmt.Errorf("host resource requires a native resource type")
			}
			if _, ok := named.Underlying().(*types.Struct); !ok {
				return nil, fmt.Errorf("host resource requires struct result")
			}
			members := map[types.Object]bool{}
			for _, name := range rule.Members {
				member, index, _ := types.LookupFieldOrMethod(pointer, true, pkg, name)
				// Embedded host objects require their own ownership proof.
				if member == nil || len(index) != 1 || member.Pkg() != named.Obj().Pkg() {
					return nil, fmt.Errorf("host resource member unavailable: %s", name)
				}
				switch member := member.(type) {
				case *types.Var:
					if !member.IsField() {
						return nil, fmt.Errorf("host resource member not a field: %s", name)
					}
				case *types.Func:
				default:
					return nil, fmt.Errorf("host resource member unsupported: %s", name)
				}
				members[member] = true
			}
			// The adapter may expose the native pointer in signatures, but no resource
			// literal, conversion, allocation, or package factory is thereby permitted.
			for id, obj := range after.Uses {
				if obj == named.Obj() && passiveHostType(id, parents) {
					reviewed[id] = true
				}
			}
			roots := map[types.Object]bool{}
			for _, f := range files {
				ast.Inspect(f, func(n ast.Node) bool {
					assign, ok := n.(*ast.AssignStmt)
					if !ok || assign.Tok != token.DEFINE || len(assign.Lhs) != 1 || len(assign.Rhs) != 1 {
						return true
					}
					id, ok := assign.Lhs[0].(*ast.Ident)
					if !ok || after.Defs[id] == nil {
						return true
					}
					call, ok := ast.Unparen(assign.Rhs[0]).(*ast.CallExpr)
					if !ok {
						return true
					}
					selector, ok := ast.Unparen(call.Fun).(*ast.SelectorExpr)
					if !ok {
						return true
					}
					selection := after.Selections[selector]
					if selection == nil || selection.Kind() != types.MethodVal || selection.Obj() != method {
						return true
					}
					if !types.Identical(after.TypeOf(id), pointer) {
						return true
					}
					roots[after.Defs[id]] = true
					return true
				})
			}
			if len(roots) == 0 {
				return nil, fmt.Errorf("host resource has no direct local factory result: %s", rule.Method)
			}
			for id, obj := range after.Uses {
				if !roots[obj] {
					continue
				}
				sel, ok := parents[id].(*ast.SelectorExpr)
				if !ok || sel.X != id {
					return nil, fmt.Errorf("host resource local escapes or is reassigned: %s", id.Name)
				}
				selection := after.Selections[sel]
				if selection == nil || !members[selection.Obj()] {
					return nil, fmt.Errorf("unreviewed host resource member: %s", sel.Sel.Name)
				}
				if selection.Kind() == types.MethodVal {
					call, ok := parents[sel].(*ast.CallExpr)
					if !ok || call.Fun != sel {
						return nil, fmt.Errorf("host resource method must be called directly: %s", sel.Sel.Name)
					}
				} else if selection.Kind() != types.FieldVal {
					return nil, fmt.Errorf("unsupported host resource selector")
				}
				reviewed[sel.Sel] = true
			}
		}
	}
	return reviewed, nil
}

func passiveHostType(id *ast.Ident, parents map[ast.Node]ast.Node) bool {
	var node ast.Node = id
	if sel, ok := parents[node].(*ast.SelectorExpr); ok && sel.Sel == id {
		node = sel
	}
	pointer, ok := parents[node].(*ast.StarExpr)
	if !ok || pointer.X != node {
		return false
	}
	node = pointer
	for {
		switch parent := parents[node].(type) {
		case *ast.Field:
			return parent.Type == node
		case *ast.ArrayType:
			if parent.Elt != node {
				return false
			}
			node = parent
		case *ast.MapType:
			if parent.Key != node && parent.Value != node {
				return false
			}
			node = parent
		case *ast.ParenExpr:
			node = parent
		default:
			return false
		}
	}
}

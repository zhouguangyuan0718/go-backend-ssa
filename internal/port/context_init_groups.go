package port

import (
	"fmt"
	"go/ast"
	"go/types"
)

// Named initializer groups are only safe when native initialization cannot
// observe state from another group. Within each group we retain Go's InitOrder
// followed by lexical init bodies. A saved function value does not execute its
// body; calls (including callbacks passed to calls) do. For a called local
// function we conservatively include all references in its body. Unknown
// indirect calls fail closed instead of assuming that they are pure.
func contextInitializerGroups(files []*ast.File, info *types.Info, units []Unit, states map[types.Object]string) ([]string, error) {
	groups := make([]string, len(files))
	active := false
	for i, unit := range units {
		if unit.Context != nil {
			groups[i] = unit.Context.InitGroup
			active = active || groups[i] != ""
		}
	}
	if !active {
		return groups, nil
	}
	stateGroups := map[types.Object]string{}
	functions := map[types.Object]*ast.FuncDecl{}
	for i, file := range files {
		for _, decl := range file.Decls {
			if fn, ok := decl.(*ast.FuncDecl); ok {
				functions[info.Defs[fn.Name]] = fn
				if units[i].Context != nil && (fn.Recv == nil || receiverType(fn) == units[i].Context.Type) {
					for _, group := range groups {
						if group != "" && group == fn.Name.Name {
							return nil, fmt.Errorf("initializer group conflicts with native method: %s", group)
						}
					}
				}
			}
		}
		for object := range states {
			if object.Pos() >= file.Pos() && object.Pos() < file.End() {
				stateGroups[object] = groups[i]
			}
		}
	}
	check := func(root ast.Node, group string) error {
		seen := map[types.Object]bool{}
		var failure error
		var inspect func(ast.Node, bool)
		inspect = func(node ast.Node, conservative bool) {
			ast.Inspect(node, func(node ast.Node) bool {
				if failure != nil {
					return false
				}
				if _, ok := node.(*ast.FuncLit); ok && !conservative {
					return false
				}
				if id, ok := node.(*ast.Ident); ok {
					object := info.Uses[id]
					if other, ok := stateGroups[object]; ok && other != group {
						failure = fmt.Errorf("initializer group %q depends on state %s in group %q", group, object.Name(), other)
						return false
					}
					if conservative && functions[object] != nil && !seen[object] {
						seen[object] = true
						if functions[object].Body == nil {
							failure = fmt.Errorf("initializer group %q calls an unavailable local body", group)
							return false
						}
						inspect(functions[object].Body, true)
					}
				}
				call, ok := node.(*ast.CallExpr)
				if !ok {
					return true
				}
				if info.Types[call.Fun].IsType() {
					return true
				}
				var object types.Object
				// A generic instantiation preserves its callee's object identity.
				// Data indexing still resolves to a variable (or no object) and
				// is rejected by the function/builtin check below.
				callee := ast.Unparen(call.Fun)
				switch indexed := callee.(type) {
				case *ast.IndexExpr:
					callee = ast.Unparen(indexed.X)
				case *ast.IndexListExpr:
					callee = ast.Unparen(indexed.X)
				}
				switch fn := callee.(type) {
				case *ast.Ident:
					object = info.Uses[fn]
				case *ast.SelectorExpr:
					if selection := info.Selections[fn]; selection != nil {
						if _, opaque := selection.Recv().Underlying().(*types.Interface); opaque {
							failure = fmt.Errorf("initializer group %q has an unresolved interface call", group)
							return false
						}
					}
					object = info.Uses[fn.Sel]
				case *ast.FuncLit:
					inspect(fn.Body, true)
					for _, arg := range call.Args {
						inspect(arg, true)
					}
					return false
				}
				switch object.(type) {
				case *types.Func, *types.Builtin:
					if fn := functions[object]; fn != nil && !seen[object] {
						seen[object] = true
						if fn.Body == nil {
							failure = fmt.Errorf("initializer group %q calls an unavailable local body", group)
							return false
						}
						inspect(fn.Body, true)
					}
				default:
					failure = fmt.Errorf("initializer group %q has an unresolved indirect call", group)
					return false
				}
				// An argument can be a callback or contain one. Include its body even
				// when the target is external and its implementation is unavailable.
				for _, arg := range call.Args {
					inspect(arg, true)
				}
				return true
			})
		}
		inspect(root, false)
		return failure
	}
	for _, entry := range info.InitOrder {
		if len(entry.Lhs) != 1 {
			return nil, fmt.Errorf("unsupported grouped state initialization")
		}
		if err := check(entry.Rhs, stateGroups[entry.Lhs[0]]); err != nil {
			return nil, err
		}
	}
	for i, file := range files {
		for _, decl := range file.Decls {
			if fn, ok := decl.(*ast.FuncDecl); ok && fn.Recv == nil && fn.Name.Name == "init" {
				if err := check(fn.Body, groups[i]); err != nil {
					return nil, err
				}
			}
		}
	}
	return groups, nil
}

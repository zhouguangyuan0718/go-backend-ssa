package port

import (
	"fmt"
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
)

func hostCallFunction(object types.Object, rule HostCallRule) bool {
	fn, ok := object.(*types.Func)
	return ok && fn.Pkg() != nil && fn.Pkg().Path() == rule.Package && fn.Name() == rule.Function && fn.Parent() == fn.Pkg().Scope()
}

func reviewedHostCalls(files []*ast.File, before, after *types.Info, pkg *types.Package, units []Unit) (map[ast.Node]bool, error) {
	reviewed := map[ast.Node]bool{}
	metrics := false
	for _, unit := range units {
		for _, rule := range unit.HostCalls {
			var original *types.Func
			for _, object := range before.Uses {
				if hostCallFunction(object, rule) {
					original = object.(*types.Func)
					break
				}
			}
			if original == nil {
				return nil, fmt.Errorf("host call not used: %s.%s", rule.Package, rule.Function)
			}
			ctx := pkg.Scope().Lookup(unit.Context.Type)
			if ctx == nil {
				return nil, fmt.Errorf("host call context missing")
			}
			object, _, _ := types.LookupFieldOrMethod(types.NewPointer(ctx.Type()), true, pkg, rule.Method)
			method, ok := object.(*types.Func)
			if !ok {
				return nil, fmt.Errorf("host call adapter missing: %s", rule.Method)
			}
			native := original.Type().(*types.Signature)
			actual := method.Type().(*types.Signature)
			want := types.TypeString(unnamedSignature(native), nil)
			if rule.Package == "os" && rule.Function == "Create" {
				// The sole signature adaptation deliberately removes File capabilities.
				// The compiler keeps its native (writer,error) flow; its factory can now
				// supply a memory stream or a host file without exposing os.File methods.
				if want != "func(string) (*os.File, error)" {
					return nil, fmt.Errorf("native create signature changed: %s", want)
				}
				want = "func(string) (io.WriteCloser, error)"
				if actual.Results().Len() != 2 {
					return nil, fmt.Errorf("host call create requires two results")
				}
				result := actual.Results().At(0).Type()
				named, ok := result.(*types.Named)
				if !ok || named.Obj().Pkg() == nil || named.Obj().Pkg().Path() != "io" || named.Obj().Name() != "WriteCloser" || !types.AssignableTo(native.Results().At(0).Type(), result) {
					return nil, fmt.Errorf("host call create requires the native writer interface")
				}
			}
			if got := types.TypeString(unnamedSignature(actual), nil); got != want {
				return nil, fmt.Errorf("host call adapter signature mismatch: %s: got %s, want %s", rule.Method, got, want)
			}
			if rule.StringArgument != "" {
				found := 0
				for _, file := range files {
					ast.Inspect(file, func(node ast.Node) bool {
						call, ok := node.(*ast.CallExpr)
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
						if len(call.Args) == 1 {
							value := after.Types[call.Args[0]].Value
							if value != nil && value.Kind() == constant.String && constant.StringVal(value) == rule.StringArgument {
								found++
							}
						}
						return true
					})
				}
				uses := 0
				for _, selection := range after.Selections {
					if selection.Obj() == method {
						uses++
					}
				}
				if found == 0 || uses != found {
					return nil, fmt.Errorf("host call requires the reviewed string argument: %s", rule.Method)
				}
			}
			metrics = metrics || rule.Package == "runtime" && rule.Function == "ReadMemStats"
		}
	}
	if !metrics {
		return reviewed, nil
	}
	parents := map[ast.Node]ast.Node{}
	for _, file := range files {
		var stack []ast.Node
		ast.Inspect(file, func(node ast.Node) bool {
			if node == nil {
				stack = stack[:len(stack)-1]
				return true
			}
			if len(stack) != 0 {
				parents[node] = stack[len(stack)-1]
			}
			stack = append(stack, node)
			return true
		})
	}
	// Provider signatures carry the original pointer type, without approving
	// field access on arbitrary buffers or direct calls into the runtime.
	for id, object := range after.Uses {
		if object != nil && object.Pkg() != nil && object.Pkg().Path() == "runtime" && object.Name() == "MemStats" && passiveHostType(id, parents) {
			if _, ok := object.(*types.TypeName); ok {
				reviewed[id] = true
			}
		}
	}
	// Only native local zero-valued counters and their two selected scalar fields
	// are passive metric data. No runtime operation is allowed by this exception.
	roots := map[types.Object]bool{}
	for _, file := range files {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			ast.Inspect(fn.Body, func(node ast.Node) bool {
				decl, ok := node.(*ast.GenDecl)
				if !ok || decl.Tok != token.VAR {
					return true
				}
				for _, spec := range decl.Specs {
					value := spec.(*ast.ValueSpec)
					if len(value.Values) != 0 {
						continue
					}
					selector, ok := value.Type.(*ast.SelectorExpr)
					if !ok {
						continue
					}
					object := after.Uses[selector.Sel]
					if object == nil || object.Pkg() == nil || object.Pkg().Path() != "runtime" || object.Name() != "MemStats" {
						continue
					}
					if _, ok := object.(*types.TypeName); !ok {
						continue
					}
					reviewed[selector.Sel] = true
					for _, name := range value.Names {
						roots[after.Defs[name]] = true
					}
				}
				return true
			})
		}
	}
	for selector, selection := range after.Selections {
		id, ok := selector.X.(*ast.Ident)
		if !ok || !roots[after.Uses[id]] || selection.Kind() != types.FieldVal {
			continue
		}
		field := selection.Obj()
		if field.Pkg() != nil && field.Pkg().Path() == "runtime" && (field.Name() == "TotalAlloc" || field.Name() == "Mallocs") {
			reviewed[selector.Sel] = true
		}
	}
	return reviewed, nil
}

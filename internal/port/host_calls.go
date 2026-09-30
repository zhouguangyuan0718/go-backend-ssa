package port

import (
	"fmt"
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"strings"
)

func (r HostCallRule) stringArguments() []string {
	if r.StringArgument != "" {
		return []string{r.StringArgument}
	}
	return r.StringArguments
}

// Imports in the pinned-source and generated-package checks have independent
// type identities. Compare canonical names after resolving parameter aliases.
func hostCallSignature(sig *types.Signature) string {
	tuple := func(in *types.Tuple) *types.Tuple {
		vars := make([]*types.Var, in.Len())
		for i := range vars {
			vars[i] = types.NewVar(token.NoPos, nil, "", types.Unalias(in.At(i).Type()))
		}
		return types.NewTuple(vars...)
	}
	return types.TypeString(types.NewSignatureType(nil, nil, nil, tuple(sig.Params()), tuple(sig.Results()), sig.Variadic()), nil)
}

func hostCallFunction(object types.Object, rule HostCallRule) bool {
	fn, ok := object.(*types.Func)
	return ok && fn.Pkg() != nil && fn.Pkg().Path() == rule.Package && fn.Name() == rule.Function && fn.Parent() == fn.Pkg().Scope()
}

func reviewedHostCalls(files []*ast.File, before, after *types.Info, pkg *types.Package, units []Unit) (map[ast.Node]bool, error) {
	reviewed := map[ast.Node]bool{}
	metrics := false
	flags := false
	for _, unit := range units {
		// Explicit scalar host inputs may also supply adapter defaults. Native
		// references are separately rewritten to the typed instance field; this
		// authorizes only the named constants, never ambient calls or variables.
		for _, rule := range unit.HostConstants {
			for id, object := range after.Uses {
				if hostConstantObject(object, rule) {
					reviewed[id] = true
				}
			}
		}

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
			want := hostCallSignature(native)
			if rule.Package == "os" && (rule.Function == "Create" || rule.Function == "Open" || rule.Function == "OpenFile") {
				// Narrow only the native file result; every parameter and the error
				// result keep their original type. No host File methods escape.
				iface := "WriteCloser"
				if rule.Function == "Open" {
					iface = "ReadCloser"
				}
				if native.Results().Len() != 2 || actual.Results().Len() != 2 || !strings.HasSuffix(want, "(*os.File, error)") {
					return nil, fmt.Errorf("host file factory signature changed: %s", want)
				}
				result := actual.Results().At(0).Type()
				named, ok := result.(*types.Named)
				allowed := ok && named.Obj().Pkg() != nil && ((named.Obj().Pkg().Path() == "io" && named.Obj().Name() == iface) || (rule.Function == "Open" && named.Obj().Pkg().Path() == "io/fs" && named.Obj().Name() == "File"))
				if len(rule.StreamMethods) != 0 {
					allowed = hostStreamResult(native.Results().At(0).Type(), result, rule.StreamMethods)
				}
				if !allowed || !hostFileImplements(native.Results().At(0).Type(), result) {
					return nil, fmt.Errorf("host file factory requires io.%s", iface)
				}
				// Type aliases such as os.FileMode and fs.FileMode are identical.
				expected := types.NewSignatureType(nil, nil, nil, native.Params(), types.NewTuple(types.NewVar(token.NoPos, nil, "", result), native.Results().At(1)), native.Variadic())
				if hostCallSignature(actual) != hostCallSignature(expected) {
					return nil, fmt.Errorf("host file factory signature mismatch: %s", rule.Method)
				}
				want = hostCallSignature(actual)
			}
			if got := hostCallSignature(actual); got != want {
				return nil, fmt.Errorf("host call adapter signature mismatch: %s: got %s, want %s", rule.Method, got, want)
			}
			if keys := rule.stringArguments(); len(keys) != 0 {
				found := 0
				counts := map[string]int{}
				for _, key := range keys {
					counts[key] = 0
				}
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
							if value != nil && value.Kind() == constant.String {
								key := constant.StringVal(value)
								if _, ok := counts[key]; ok {
									found++
									counts[key]++
								}
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
				for _, count := range counts {
					if count == 0 {
						return nil, fmt.Errorf("host call reviewed string argument is unused: %s", rule.Method)
					}
				}
				if found == 0 || uses != found {
					return nil, fmt.Errorf("host call requires the reviewed string argument: %s", rule.Method)
				}
			}
			if rule.Package == "os" && rule.Function == "OpenFile" {
				// Only immutable native open flags used in this factory's flags
				// argument pass the host audit. No other os use is authorized.
				for _, file := range files {
					ast.Inspect(file, func(node ast.Node) bool {
						call, ok := node.(*ast.CallExpr)
						if !ok || len(call.Args) != 3 {
							return true
						}
						sel, ok := ast.Unparen(call.Fun).(*ast.SelectorExpr)
						if !ok {
							return true
						}
						selection := after.Selections[sel]
						if selection == nil || selection.Kind() != types.MethodVal || selection.Obj() != method {
							return true
						}
						ast.Inspect(call.Args[1], func(n ast.Node) bool {
							id, ok := n.(*ast.Ident)
							if !ok {
								return true
							}
							obj, ok := after.Uses[id].(*types.Const)
							if ok && obj.Pkg() != nil && obj.Pkg().Path() == "os" && (obj.Name() == "O_WRONLY" || obj.Name() == "O_CREATE" || obj.Name() == "O_TRUNC") {
								reviewed[id] = true
							}
							return true
						})
						return true
					})
				}
			}
			flags = flags || rule.Package == "flag" && rule.Function == "Lookup"
			metrics = metrics || rule.Package == "runtime" && rule.Function == "ReadMemStats"
		}
	}

	if flags {
		// An explicit FlagSet lookup returns passive flag descriptions. Keep only
		// their value/name/default and read-only Getter operations; process parsing,
		// global registration and Value.Set remain unclassified.
		for id, object := range after.Uses {
			if object == nil || object.Pkg() == nil || object.Pkg().Path() != "flag" {
				continue
			}
			if _, ok := object.(*types.TypeName); ok && (object.Name() == "Flag" || object.Name() == "Getter") {
				reviewed[id] = true
			}
		}
		for selector, selection := range after.Selections {
			object := selection.Obj()
			if object.Pkg() == nil || object.Pkg().Path() != "flag" {
				continue
			}
			recv := selection.Recv()
			if p, ok := recv.(*types.Pointer); ok {
				recv = p.Elem()
			}
			named, ok := recv.(*types.Named)
			if !ok || named.Obj().Pkg() == nil || named.Obj().Pkg().Path() != "flag" {
				continue
			}
			switch named.Obj().Name() {
			case "Flag":
				if selection.Kind() == types.FieldVal && (object.Name() == "Value" || object.Name() == "Name" || object.Name() == "DefValue") {
					reviewed[selector.Sel] = true
				}
			case "Getter", "Value":
				if (selection.Kind() == types.MethodVal || selection.Kind() == types.MethodExpr) && (object.Name() == "String" || object.Name() == "Get") {
					reviewed[selector.Sel] = true
				}
			}
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

// The two importers give fs.FileInfo independent named identities. Compare the
// canonical signatures of the already allowlisted interface's complete method set.
func hostFileImplements(native, narrowed types.Type) bool {
	iface, ok := narrowed.Underlying().(*types.Interface)
	if !ok {
		return false
	}
	iface.Complete()
	for i := 0; i < iface.NumMethods(); i++ {
		want := iface.Method(i)
		got, _, _ := types.LookupFieldOrMethod(native, true, nil, want.Name())
		fn, ok := got.(*types.Func)
		if !ok || hostCallSignature(fn.Type().(*types.Signature)) != hostCallSignature(want.Type().(*types.Signature)) {
			return false
		}
	}
	return true
}

package port

import (
	"fmt"
	"go/ast"
	"go/types"
	"path"
	"strings"
)

func nativeMethodObject(obj types.Object) bool {
	f, ok := obj.(*types.Func)
	if !ok {
		return false
	}
	return f.Type().(*types.Signature).Recv() != nil
}
func originalBoundMethod(before *packageImporter, r MethodCallRule) (*types.Func, error) {
	pkg := before.packages[strings.TrimPrefix(path.Dir(r.Source), "src/")]
	typ, name, _ := strings.Cut(r.Method, ".")
	if pkg == nil || pkg.Scope().Lookup(typ) == nil {
		return nil, fmt.Errorf("method dependency is not migrated: %s", r.Method)
	}
	obj, _, _ := types.LookupFieldOrMethod(types.NewPointer(pkg.Scope().Lookup(typ).Type()), true, pkg, name)
	f, ok := obj.(*types.Func)
	if !ok || before.sources[f] != r.Source {
		return nil, fmt.Errorf("method source mismatch: %s", r.Method)
	}
	sig := f.Type().(*types.Signature)
	if sig.Recv() == nil || sig.RecvTypeParams().Len() != 0 || sig.TypeParams().Len() != 0 || before.recovering[f] {
		return nil, fmt.Errorf("unsupported method binding: %s", r.Method)
	}
	return f, nil
}
func loadMethodCallSignatures(s *Source, units []Unit, before *packageImporter, hashes map[string]string) error {
	for _, u := range units {
		for _, r := range u.MethodCalls {
			if _, e := originalBoundMethod(before, r); e != nil {
				return e
			}
			b, e := s.Read(r.Source)
			if e != nil {
				return e
			}
			hashes[r.Source] = hash(b)
		}
	}
	return nil
}
func methodCallBindings(files []*ast.File, info *types.Info, units []Unit) (map[types.Object]string, error) {
	out := map[types.Object]string{}
	calls := map[*ast.SelectorExpr]bool{}
	for _, f := range files {
		ast.Inspect(f, func(n ast.Node) bool {
			if call, ok := n.(*ast.CallExpr); ok {
				if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
					calls[sel] = true
				}
			}
			return true
		})
	}
	for i, u := range units {
		for _, r := range u.MethodCalls {
			typ, name, _ := strings.Cut(r.Method, ".")
			pkg := strings.TrimPrefix(path.Dir(r.Source), "src/")
			found := false
			for sel, selection := range info.Selections {
				if sel.Pos() < files[i].Pos() || sel.Pos() >= files[i].End() {
					continue
				}
				f, ok := selection.Obj().(*types.Func)
				if !ok || f.Pkg() == nil || f.Pkg().Path() != pkg || f.Name() != name {
					continue
				}
				sig := f.Type().(*types.Signature)
				recv := sig.Recv().Type()
				base := recv
				if p, ok := base.(*types.Pointer); ok {
					base = p.Elem()
				}
				named, ok := base.(*types.Named)
				if !ok || named.Obj().Name() != typ {
					continue
				}
				if prev := out[f]; prev != "" && prev != r.Adapter {
					return nil, fmt.Errorf("conflicting method binding: %s", r.Method)
				}
				out[f] = r.Adapter
				found = true
			}
			if !found {
				return nil, fmt.Errorf("bound method is unused: %s", r.Method)
			}
		}
	}
	// Bindings propagate through the selected package's call graph. Validate
	// every use, including files that inherit a binding from another unit.
	for sel, selection := range info.Selections {
		if out[selection.Obj()] == "" {
			continue
		}
		sig := selection.Obj().Type().(*types.Signature)
		if selection.Kind() != types.MethodVal || !calls[sel] || len(selection.Index()) != 1 || !types.Identical(info.TypeOf(sel.X), sig.Recv().Type()) {
			return nil, fmt.Errorf("method binding requires a direct exact receiver call: %s", selection.Obj().Name())
		}
	}
	return out, nil
}
func rewriteBoundMethodCalls(node ast.Node, info *types.Info, bindings map[types.Object]string, ctx ast.Expr) {
	ast.Inspect(node, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		target := bindings[info.Uses[sel.Sel]]
		if target == "" {
			return true
		}
		var fun ast.Expr = positionedContext(ctx, sel.Pos())
		for _, part := range strings.Split(target, ".") {
			fun = &ast.SelectorExpr{X: fun, Sel: &ast.Ident{Name: part, NamePos: sel.Pos()}}
		}
		call.Fun = fun
		call.Args = append([]ast.Expr{sel.X}, call.Args...)
		return true
	})
}
func checkMethodCallAdapters(units []Unit, before *packageImporter, out *types.Package, imports map[string]string) error {
	for _, u := range units {
		for _, r := range u.MethodCalls {
			original, e := originalBoundMethod(before, r)
			if e != nil {
				return e
			}
			ctx := out.Scope().Lookup(u.Context.Type)
			if ctx == nil {
				return fmt.Errorf("missing method adapter context")
			}
			adapter, _, _ := lookupAdapterMember(types.NewPointer(ctx.Type()), out, r.Adapter)
			f, ok := adapter.(*types.Func)
			if !ok {
				return fmt.Errorf("missing method adapter: %s", r.Adapter)
			}
			sig := original.Type().(*types.Signature)
			params := []*types.Var{types.NewParam(0, nil, "", sig.Recv().Type())}
			for i := 0; i < sig.Params().Len(); i++ {
				params = append(params, sig.Params().At(i))
			}
			wantSig := types.NewSignatureType(nil, nil, nil, types.NewTuple(params...), sig.Results(), sig.Variadic())
			want := types.TypeString(unnamedSignature(wantSig), func(p *types.Package) string {
				if to := imports[p.Path()]; to != "" {
					return to
				}
				return p.Path()
			})
			got := types.TypeString(unnamedSignature(f.Type()), nil)
			if got != want {
				return fmt.Errorf("method adapter signature mismatch for %s: got %s, want %s", r.Adapter, got, want)
			}
		}
	}
	return nil
}

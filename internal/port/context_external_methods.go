package port

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"path"
	"strings"
)

func contextMethodCallerPackage(file *ast.File, info *types.Info) *types.Package {
	for id, obj := range info.Defs {
		if id.Pos() >= file.Pos() && id.Pos() < file.End() && obj != nil && obj.Pkg() != nil {
			return obj.Pkg()
		}
	}
	return nil
}

func externalContextMethods(files []*ast.File, info *types.Info, units []Unit) (map[types.Object]ContextParamCallRule, error) {
	out := map[types.Object]ContextParamCallRule{}
	for i, u := range units {
		for _, r := range u.ContextParamCalls {
			typ, method, _ := strings.Cut(r.Method, ".")
			pkg := strings.TrimPrefix(path.Dir(r.Source), "src/")
			found := false
			for sel, selection := range info.Selections {
				if sel.Pos() < files[i].Pos() || sel.Pos() >= files[i].End() {
					continue
				}
				obj := selection.Obj()
				sig, ok := obj.Type().(*types.Signature)
				if !ok || sig.Recv() == nil || obj.Pkg() == nil || obj.Pkg().Path() != pkg || obj.Name() != method {
					continue
				}
				recv := sig.Recv().Type()
				if p, ok := recv.(*types.Pointer); ok {
					recv = p.Elem()
				}
				named, ok := recv.(*types.Named)
				if !ok || named.Obj().Name() != typ {
					continue
				}
				if old, ok := out[obj]; ok && old != r {
					return nil, fmt.Errorf("conflicting external context method: %s", r.Method)
				}
				out[obj] = r
				found = true
			}
			if !found {
				return nil, fmt.Errorf("external context method not used: %s", r.Method)
			}
		}
	}
	return out, nil
}

func threadExternalMethodContexts(fs *token.FileSet, files []*ast.File, node ast.Node, info *types.Info, methods map[types.Object]ContextParamCallRule, local map[types.Object]bool, ctx ast.Expr, localType string, recovering map[types.Object]bool) error {
	bindings := map[types.Object]contextMethodBinding{}
	for obj := range local {
		bindings[obj] = contextMethodBinding{expr: ctx, typeName: localType}
	}
	pos := token.NoPos
	ast.Inspect(node, func(n ast.Node) bool {
		if sel, ok := n.(*ast.SelectorExpr); ok {
			if _, ok := methods[info.Uses[sel.Sel]]; ok {
				pos = sel.Pos()
			}
		}
		return true
	})
	if pos == token.NoPos {
		return threadContextMethodBindings(fs, files, node, info, bindings)
	}
	for _, file := range files {
		if pos < file.Pos() || pos >= file.End() {
			continue
		}
		qualifier := typeQualifier(file, info, contextMethodCallerPackage(file, info))
		for obj, r := range methods {
			used := false
			ast.Inspect(node, func(n ast.Node) bool {
				if s, ok := n.(*ast.SelectorExpr); ok && info.Uses[s.Sel] == obj {
					used = true
				}
				return true
			})
			if !used {
				continue
			}
			contextType := r.ContextType
			if prefix := qualifier(obj.Pkg()); prefix != "" {
				contextType = prefix + "." + contextType
			}
			bound := positionedContext(ctx, pos)
			for _, part := range strings.Split(r.Field, ".") {
				bound = &ast.SelectorExpr{X: bound, Sel: &ast.Ident{Name: part, NamePos: pos}}
			}
			bindings[obj] = contextMethodBinding{expr: bound, typeName: contextType, recovers: recovering[obj]}
		}
		return threadContextMethodBindings(fs, files, node, info, bindings)
	}
	return fmt.Errorf("external context method node has no source file")
}

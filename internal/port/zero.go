package port

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
)

// replaceZeroValues removes only explicitly reviewed, uninitialized struct
// sentinels. Each use receives a fresh value (or fresh allocation for &sentinel).
// In particular, a nil PosBase must not return a shared mutable *Pos.
func replaceZeroValues(fs *token.FileSet, files []*ast.File, units []Unit, info *types.Info) error {
	zeros := map[types.Object]string{}
	for i, u := range units {
		if len(u.ZeroValues) == 0 {
			continue
		}
		cm := ast.NewCommentMap(fs, files[i], files[i].Comments)
		wanted := map[string]bool{}
		for _, name := range u.ZeroValues {
			if wanted[name] {
				return fmt.Errorf("duplicate zero-value rule: %s", name)
			}
			wanted[name] = true
		}
		found := map[string]bool{}
		var decls []ast.Decl
		for _, d := range files[i].Decls {
			g, ok := d.(*ast.GenDecl)
			if !ok || g.Tok != token.VAR {
				decls = append(decls, d)
				continue
			}
			var specs []ast.Spec
			for _, spec := range g.Specs {
				v := spec.(*ast.ValueSpec)
				remove := false
				for _, id := range v.Names {
					if !wanted[id.Name] {
						continue
					}
					if len(v.Names) != 1 || len(v.Values) != 0 {
						return fmt.Errorf("zero-value rule requires an uninitialized single variable: %s", id.Name)
					}
					obj := info.Defs[id]
					named, ok := obj.Type().(*types.Named)
					if !ok {
						return fmt.Errorf("zero-value rule requires a named struct: %s", id.Name)
					}
					if _, ok := named.Underlying().(*types.Struct); !ok {
						return fmt.Errorf("zero-value rule requires a struct: %s", id.Name)
					}
					name := zeroTypeName(obj)
					if named.Obj().Pkg() != obj.Pkg() {
						// A package-local alias preserves the imported type's
						// identity without adding imports in every consuming file.
						for _, f := range files {
							collision := false
							ast.Inspect(f, func(n ast.Node) bool {
								if id, ok := n.(*ast.Ident); ok && id.Name == name {
									collision = true
								}
								return true
							})
							if collision {
								return fmt.Errorf("zero type alias collision: %s", name)
							}
						}
						decls = append(decls, &ast.GenDecl{TokPos: g.TokPos, Tok: token.TYPE, Specs: []ast.Spec{&ast.TypeSpec{Name: &ast.Ident{Name: name, NamePos: id.Pos()}, Assign: id.Pos(), Type: v.Type}}})
					}
					zeros[obj] = name
					found[id.Name], remove = true, true
				}
				if !remove {
					specs = append(specs, spec)
				}
			}
			if len(specs) != 0 {
				g.Specs = specs
				decls = append(decls, g)
			}
		}
		for name := range wanted {
			if !found[name] {
				return fmt.Errorf("zero-value declaration not found: %s", name)
			}
		}
		files[i].Decls = decls
		files[i].Comments = cm.Filter(files[i]).Comments()
	}
	if len(zeros) == 0 {
		return nil
	}
	addresses := zeroMethodReceivers(info)
	replacements := map[*ast.Ident]ast.Expr{}
	for id, obj := range info.Uses {
		if name := zeros[obj]; name != "" {
			// Parentheses disambiguate a composite literal in if/switch headers.
			replacements[id] = freshZero(&ast.Ident{Name: name, NamePos: id.Pos()}, addresses[id])
		}
	}
	for _, f := range files {
		rewriteNodes(f, replacements)
	}
	return nil
}

// Uses of another migrated package's zero sentinel follow the declaration's
// reviewed rule; consumers do not need a second per-file replacement list.
func replaceImportedZeros(files []*ast.File, info *types.Info, zeros map[types.Object]string) {
	addresses := zeroMethodReceivers(info)
	for _, f := range files {
		replacements := map[ast.Node]ast.Expr{}
		ast.Inspect(f, func(n ast.Node) bool {
			if sel, ok := n.(*ast.SelectorExpr); ok {
				if name := zeros[info.Uses[sel.Sel]]; name != "" {
					replacements[sel] = freshZero(&ast.SelectorExpr{X: sel.X, Sel: &ast.Ident{Name: name, NamePos: sel.Sel.Pos()}}, addresses[sel])
				}
			}
			return true
		})
		rewriteExprs(f, replacements)
	}
}

// Imported zero types get an alias in the sentinel's original package. Export
// it only when cross-package uses of an exported sentinel need to name it.
func zeroTypeName(obj types.Object) string {
	named := obj.Type().(*types.Named)
	if named.Obj().Pkg() == obj.Pkg() {
		return named.Obj().Name()
	}
	prefix := "zeroValue_"
	if obj.Exported() {
		prefix = "ZeroValue_"
	}
	return prefix + obj.Name()
}

func zeroMethodReceivers(info *types.Info) map[ast.Expr]bool {
	addresses := map[ast.Expr]bool{}
	for sel, selection := range info.Selections {
		sig, ok := selection.Obj().Type().(*types.Signature)
		if !ok || sig.Recv() == nil {
			continue
		}
		if _, ok := sig.Recv().Type().(*types.Pointer); ok {
			addresses[ast.Unparen(sel.X)] = true
		}
	}
	return addresses
}
func freshZero(typ ast.Expr, addressable bool) ast.Expr {
	var value ast.Expr = &ast.CompositeLit{Type: typ}
	if addressable {
		value = &ast.StarExpr{X: &ast.UnaryExpr{Op: token.AND, X: value}}
	}
	return &ast.ParenExpr{X: value}
}

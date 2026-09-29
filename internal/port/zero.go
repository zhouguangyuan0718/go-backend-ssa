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
					if !ok || named.Obj().Pkg() != obj.Pkg() {
						return fmt.Errorf("zero-value rule requires a local named struct: %s", id.Name)
					}
					if _, ok := named.Underlying().(*types.Struct); !ok {
						return fmt.Errorf("zero-value rule requires a struct: %s", id.Name)
					}
					zeros[obj] = named.Obj().Name()
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
	replacements := map[*ast.Ident]ast.Expr{}
	for id, obj := range info.Uses {
		if name := zeros[obj]; name != "" {
			// Parentheses disambiguate a composite literal in if/switch headers.
			replacements[id] = &ast.ParenExpr{X: &ast.CompositeLit{Type: &ast.Ident{Name: name, NamePos: id.Pos()}}}
		}
	}
	for _, f := range files {
		rewriteNodes(f, replacements)
	}
	return nil
}

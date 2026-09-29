package port

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"strconv"
)

// A generated type expression must use the consuming file's import names, not
// names from the declaration's file. New imports use collision-checked reserved
// names and are relocated together with the rest of the source.
func typeQualifier(file *ast.File, info *types.Info, pkg *types.Package) types.Qualifier {
	names := map[string]string{}
	for _, im := range file.Imports {
		p, _ := strconv.Unquote(im.Path.Value)
		if im.Name != nil {
			if im.Name.Name != "_" {
				name := im.Name.Name
				if name == "." {
					name = ""
				}
				names[p] = name
			}
			continue
		}
		if pn, ok := info.Implicits[im].(*types.PkgName); ok {
			names[p] = pn.Name()
		}
	}
	return func(p *types.Package) string {
		if p == pkg {
			return ""
		}
		if name, ok := names[p.Path()]; ok {
			return name
		}
		name := fmt.Sprintf("syncgoTypeImport%d", len(names))
		names[p.Path()] = name
		im := &ast.ImportSpec{Name: &ast.Ident{Name: name, NamePos: file.Name.End()}, Path: &ast.BasicLit{Kind: token.STRING, ValuePos: file.Name.End(), Value: strconv.Quote(p.Path())}}
		file.Imports = append(file.Imports, im)
		file.Decls = append([]ast.Decl{&ast.GenDecl{Tok: token.IMPORT, TokPos: file.Name.End(), Specs: []ast.Spec{im}}}, file.Decls...)
		return name
	}
}

package port

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"reflect"
	"strconv"
)

// Extend an upstream context instead of maintaining a second, parallel type.
// Type syntax and imports come from the already-checked source declarations.
func extendNativeContext(files []*ast.File, info *types.Info, units []Unit) error {
	var context *ContextRule
	fields := map[string]types.Type{}
	for _, u := range units {
		if u.Context == nil || !u.Context.Existing {
			continue
		}
		context = u.Context
		for source, field := range context.Fields {
			for id, obj := range info.Defs {
				if id.Name == source && obj != nil && obj.Pkg() != nil && obj.Parent() == obj.Pkg().Scope() {
					fields[field] = obj.Type()
				}
			}
			if fields[field] == nil {
				return fmt.Errorf("native context state missing: %s", source)
			}
		}
	}
	if context == nil {
		return nil
	}
	var structure *ast.StructType
	var file *ast.File
	var pkg *types.Package
	constructors := 0
	for _, f := range files {
		for _, d := range f.Decls {
			if fn, ok := d.(*ast.FuncDecl); ok && fn.Recv == nil && fn.Name.Name == "syncgoNewContext" {
				constructors++
			}
			if g, ok := d.(*ast.GenDecl); ok && g.Tok == token.TYPE {
				for _, spec := range g.Specs {
					if s := spec.(*ast.TypeSpec); s.Name.Name == context.Type {
						structure, _ = s.Type.(*ast.StructType)
						file = f
						pkg = info.Defs[s.Name].Pkg()
					}
				}
			}
		}
	}
	if structure == nil || constructors != 1 {
		return fmt.Errorf("native context requires its struct and one original constructor")
	}
	if context.Embed != "" {
		for _, field := range structure.Fields.List {
			for _, id := range field.Names {
				if id.Name == context.Embed {
					return fmt.Errorf("native context embed collision: %s", context.Embed)
				}
			}
			if id, ok := field.Type.(*ast.Ident); ok && len(field.Names) == 0 && id.Name == context.Embed {
				return fmt.Errorf("native context embed collision: %s", context.Embed)
			}
		}
		structure.Fields.List = append(structure.Fields.List, &ast.Field{Type: &ast.Ident{Name: context.Embed, NamePos: structure.Fields.Closing}})
	}
	names := map[string]string{}
	for _, im := range file.Imports {
		p, _ := strconv.Unquote(im.Path.Value)
		name := ""
		if im.Name != nil {
			name = im.Name.Name
		} else if obj, ok := info.Implicits[im].(*types.PkgName); ok {
			name = obj.Name()
		}
		// The source check stores explicit aliases in Defs; unnamed imports use
		// their package name, which can be recovered from any imported identifier.
		if name == "" {
			for _, obj := range info.Uses {
				if pn, ok := obj.(*types.PkgName); ok && pn.Imported().Path() == p {
					name = pn.Name()
					break
				}
			}
		}
		if name != "" && name != "_" {
			if name == "." {
				name = ""
			}
			names[p] = name
		}
	}
	var added []*ast.ImportSpec
	qualifier := func(p *types.Package) string {
		if p == pkg {
			return ""
		}
		if name, ok := names[p.Path()]; ok {
			return name
		}
		name := fmt.Sprintf("syncgoImport%d", len(added))
		names[p.Path()] = name
		im := &ast.ImportSpec{Name: &ast.Ident{Name: name, NamePos: file.Name.End()}, Path: &ast.BasicLit{ValuePos: file.Name.End(), Kind: token.STRING, Value: strconv.Quote(p.Path())}}
		added = append(added, im)
		return name
	}
	for _, name := range sortedKeys(fields) {
		for _, f := range structure.Fields.List {
			for _, id := range f.Names {
				if id.Name == name {
					return fmt.Errorf("native context field collision: %s", name)
				}
			}
		}
		typ, err := parser.ParseExpr(types.TypeString(fields[name], qualifier))
		if err != nil {
			return err
		}
		setTokenPositions(reflect.ValueOf(typ), structure.Fields.Closing)
		structure.Fields.List = append(structure.Fields.List, &ast.Field{Names: []*ast.Ident{{Name: name, NamePos: structure.Fields.Closing}}, Type: typ})
	}
	if len(added) > 0 {
		specs := make([]ast.Spec, len(added))
		for i, im := range added {
			specs[i] = im
		}
		file.Decls = append([]ast.Decl{&ast.GenDecl{TokPos: file.Name.End(), Tok: token.IMPORT, Specs: specs}}, file.Decls...)
		file.Imports = append(file.Imports, added...)
	}
	return nil
}

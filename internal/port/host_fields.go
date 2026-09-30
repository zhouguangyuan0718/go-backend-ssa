package port

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
)

// rewriteHostFields narrows only explicitly reviewed private os.File fields to
// their required stream operations. Native buffering and seek bodies survive.
func rewriteHostFields(files []*ast.File, info *types.Info, units []Unit) error {
	for i, u := range units {
		for _, rule := range u.HostFields {
			found := false
			for _, decl := range files[i].Decls {
				gen, ok := decl.(*ast.GenDecl)
				if !ok || gen.Tok != token.TYPE {
					continue
				}
				for _, spec := range gen.Specs {
					ts := spec.(*ast.TypeSpec)
					if ts.Name.Name != rule.Type {
						continue
					}
					st, ok := ts.Type.(*ast.StructType)
					if !ok {
						return fmt.Errorf("host stream owner is not a struct: %s", rule.Type)
					}
					for _, field := range st.Fields.List {
						if len(field.Names) != 1 || field.Names[0].Name != rule.Field {
							continue
						}
						ptr, ok := info.TypeOf(field.Type).(*types.Pointer)
						if !ok {
							return fmt.Errorf("host stream field is not a pointer: %s.%s", rule.Type, rule.Field)
						}
						named, ok := ptr.Elem().(*types.Named)
						if !ok || named.Obj().Pkg() == nil || named.Obj().Pkg().Path() != "os" || named.Obj().Name() != "File" {
							return fmt.Errorf("host stream field must be *os.File: %s.%s", rule.Type, rule.Field)
						}
						methods := &ast.FieldList{}
						for _, name := range rule.Methods {
							obj, index, _ := types.LookupFieldOrMethod(ptr, true, nil, name)
							fn, ok := obj.(*types.Func)
							if !ok || len(index) != 1 {
								return fmt.Errorf("host stream method not available: %s", name)
							}
							typ, err := parser.ParseExpr(types.TypeString(unnamedSignature(fn.Type()), func(p *types.Package) string { return p.Path() }))
							if err != nil {
								return fmt.Errorf("host stream signature %s: %w", name, err)
							}
							// The reviewed stream members use only builtin scalar/slice/error types.
							foreign := false
							ast.Inspect(typ, func(n ast.Node) bool {
								if _, ok := n.(*ast.SelectorExpr); ok {
									foreign = true
								}
								return true
							})
							if foreign {
								return fmt.Errorf("host stream method has a nonportable signature: %s", name)
							}
							methods.List = append(methods.List, &ast.Field{Names: []*ast.Ident{ast.NewIdent(name)}, Type: typ})
						}
						field.Type = &ast.InterfaceType{Methods: methods}
						found = true
					}
				}
			}
			if !found {
				return fmt.Errorf("host stream field not selected: %s.%s", rule.Type, rule.Field)
			}
		}
	}
	return nil
}

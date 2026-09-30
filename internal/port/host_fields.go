package port

import (
	"fmt"
	"go/ast"
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
						typ, err := hostStreamInterface(info.TypeOf(field.Type), rule.Methods)
						if err != nil {
							return fmt.Errorf("host stream field %s.%s: %w", rule.Type, rule.Field, err)
						}
						field.Type = typ
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

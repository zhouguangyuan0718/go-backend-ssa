package port

import (
	"fmt"
	"go/ast"
	"go/types"
)

func contextSourceFunctions(files []*ast.File, info *types.Info, units []Unit) (map[types.Object]ContextSource, error) {
	found := map[types.Object]ContextSource{}
	for i, u := range units {
		for name, source := range u.ContextFrom {
			selected := false
			for _, d := range files[i].Decls {
				fn, ok := d.(*ast.FuncDecl)
				if !ok {
					continue
				}
				key := fn.Name.Name
				if fn.Recv != nil {
					key = receiverType(fn) + "." + key
				}
				if key != name {
					continue
				}
				if fn.Body == nil {
					return nil, fmt.Errorf("context source requires function body: %s", name)
				}
				matched := false
				for _, list := range []*ast.FieldList{fn.Recv, fn.Type.Params} {
					if list != nil {
						for _, f := range list.List {
							for _, id := range f.Names {
								matched = matched || id.Name == source.Argument
							}
						}
					}
				}
				if !matched {
					return nil, fmt.Errorf("context source argument not found: %s.%s", name, source.Argument)
				}
				found[info.Defs[fn.Name]] = source
				selected = true
			}
			if !selected {
				return nil, fmt.Errorf("context source function not selected: %s", name)
			}
		}
	}
	return found, nil
}

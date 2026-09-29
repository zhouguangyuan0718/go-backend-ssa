package port

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"strings"
)

// Check the actual native body and receiver first, then omit only explicitly
// reviewed method definitions. Calls keep their original method identity.
func removeAdapterMethods(files []*ast.File, units []Unit) error {
	for i, u := range units {
		wanted := map[string]bool{}
		for _, name := range u.AdapterMethods {
			wanted[name] = true
		}
		var decls []ast.Decl
		for _, d := range files[i].Decls {
			fn, ok := d.(*ast.FuncDecl)
			if ok && fn.Recv != nil {
				key := receiverType(fn) + "." + fn.Name.Name
				if wanted[key] {
					delete(wanted, key)
					continue
				}
			}
			decls = append(decls, d)
		}
		for name := range wanted {
			return fmt.Errorf("adapter method not selected: %s", name)
		}
		files[i].Decls = decls
	}
	return nil
}

func checkAdapterMethods(units []Unit, original, output *types.Package, imports map[string]string, fs *token.FileSet) error {
	qualify := func(p *types.Package) string {
		if dst := imports[p.Path()]; dst != "" {
			return dst
		}
		return p.Path()
	}
	for _, u := range units {
		for _, name := range u.AdapterMethods {
			receiver, method, _ := strings.Cut(name, ".")
			oldType, newType := original.Scope().Lookup(receiver), output.Scope().Lookup(receiver)
			if oldType == nil || newType == nil {
				return fmt.Errorf("adapter method receiver missing: %s", name)
			}
			old, _, _ := types.LookupFieldOrMethod(types.NewPointer(oldType.Type()), true, original, method)
			actual, _, _ := types.LookupFieldOrMethod(types.NewPointer(newType.Type()), true, output, method)
			oldFn, oldOK := old.(*types.Func)
			newFn, newOK := actual.(*types.Func)
			if !oldOK || !newOK || fs.Position(newFn.Pos()).Filename != u.adapterPath() {
				return fmt.Errorf("adapter method must be declared in %s: %s", u.adapterPath(), name)
			}
			want, got := oldFn.Type().(*types.Signature), newFn.Type().(*types.Signature)
			if types.TypeString(want.Recv().Type(), qualify) != types.TypeString(got.Recv().Type(), nil) || types.TypeString(unnamedSignature(want), qualify) != types.TypeString(unnamedSignature(got), nil) {
				return fmt.Errorf("adapter method signature mismatch: %s", name)
			}
		}
	}
	return nil
}

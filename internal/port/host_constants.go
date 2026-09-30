package port

import (
	"fmt"
	"go/types"
)

func hostConstantObject(obj types.Object, r HostConstantRule) bool {
	c, ok := obj.(*types.Const)
	return ok && c.Pkg() != nil && c.Pkg().Path() == r.Package && c.Name() == r.Constant && c.Parent() == c.Pkg().Scope()
}

func checkHostConstants(units []Unit, info *types.Info, out *types.Package) error {
	for _, u := range units {
		for _, r := range u.HostConstants {
			var original types.Object
			for _, obj := range info.Uses {
				if hostConstantObject(obj, r) {
					original = obj
					break
				}
			}
			if original == nil {
				return fmt.Errorf("host constant not used: %s.%s", r.Package, r.Constant)
			}
			ctx := out.Scope().Lookup(u.Context.Type)
			if ctx == nil {
				return fmt.Errorf("missing host constant context")
			}
			member, _, _ := lookupAdapterMember(types.NewPointer(ctx.Type()), out, r.Field)
			field, ok := member.(*types.Var)
			if !ok || !field.IsField() || !types.Identical(field.Type(), types.Default(original.Type())) {
				return fmt.Errorf("host constant type mismatch: %s", r.Field)
			}
		}
	}
	return nil
}

// Compare each method against the standard library's original *os.File and
// require exactly the reviewed set, including through embedded interfaces.
func hostStreamResult(native, result types.Type, names []string) bool {
	iface, ok := result.Underlying().(*types.Interface)
	if !ok {
		return false
	}
	iface.Complete()
	if iface.NumMethods() != len(names) || !iface.IsMethodSet() {
		return false
	}
	for _, name := range names {
		original, index, _ := types.LookupFieldOrMethod(native, true, nil, name)
		fn, ok := original.(*types.Func)
		if !ok || len(index) != 1 {
			return false
		}
		var actual *types.Func
		for i := 0; i < iface.NumMethods(); i++ {
			if iface.Method(i).Name() == name {
				actual = iface.Method(i)
				break
			}
		}
		if actual == nil || hostCallSignature(fn.Type().(*types.Signature)) != hostCallSignature(actual.Type().(*types.Signature)) {
			return false
		}
	}
	return true
}

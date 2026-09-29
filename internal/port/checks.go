package port

import (
	"fmt"
	"go/types"
	"strings"
)

// Validators take the exact upstream parameter list and return nothing. This
// prevents argument forwarding or permissive adapter types from hiding drift.
func checkEntryAdapters(units []Unit, original, output *types.Package, imports map[string]string) error {
	for _, u := range units {
		for _, name := range sortedKeys(u.Checks) {
			method := u.Checks[name]
			var obj types.Object
			if receiver, member, ok := strings.Cut(name, "."); ok {
				typ := original.Scope().Lookup(receiver)
				if typ != nil {
					obj, _, _ = types.LookupFieldOrMethod(types.NewPointer(typ.Type()), true, original, member)
				}
			} else {
				obj = original.Scope().Lookup(name)
			}
			if obj == nil {
				return fmt.Errorf("missing checked declaration %s", name)
			}
			signature, ok := obj.Type().(*types.Signature)
			if !ok {
				return fmt.Errorf("check target is not a function: %s", name)
			}
			expected := types.NewSignatureType(nil, nil, nil, signature.Params(), types.NewTuple(), signature.Variadic())
			ctx := output.Scope().Lookup(u.Context.Type)
			if ctx == nil {
				return fmt.Errorf("missing validator context")
			}
			actual, _, _ := types.LookupFieldOrMethod(types.NewPointer(ctx.Type()), true, output, method)
			if _, ok := actual.(*types.Func); !ok {
				return fmt.Errorf("missing validator %s", method)
			}
			want := types.TypeString(expected, func(p *types.Package) string {
				if dst := imports[p.Path()]; dst != "" {
					return dst
				}
				return p.Path()
			})
			if got := types.TypeString(actual.Type(), nil); got != want {
				return fmt.Errorf("validator signature mismatch for %s: got %s, want %s", method, got, want)
			}
		}
	}
	return nil
}

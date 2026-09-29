package port

import (
	"go/ast"
	"go/types"
)

// Keep pure upstream helpers as free functions. Resolve dependencies by object
// identity, including function values captured by closures and initializers.
// Reviewed parameter methods participate in the same fixed point. Other
// receiver methods obtain state from their receiver and keep their signature.
func statefulFunctions(files []*ast.File, info *types.Info, candidates, states, external map[types.Object]string, owners []*OwnerRule, units []Unit, literals map[ast.Node]string) map[types.Object]string {
	needed := map[types.Object]string{}
	deps := map[types.Object][]types.Object{}
	for i, f := range files {
		for _, d := range f.Decls {
			fn, ok := d.(*ast.FuncDecl)
			if !ok {
				continue
			}
			obj := info.Defs[fn.Name]
			if candidates[obj] == "" {
				continue
			}
			key := fn.Name.Name
			if fn.Recv != nil {
				key = receiverType(fn) + "." + key
			}
			if hasOwnedAllocation(fn.Body, info, owners) || hasBoundLiteral(fn.Body, literals) || units[i].Checks[key] != "" || len(units[i].Nullable[key]) != 0 {
				needed[obj] = candidates[obj]
			}
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				if id, ok := n.(*ast.Ident); ok {
					dep := info.Uses[id]
					if states[dep] != "" || external[dep] != "" {
						needed[obj] = candidates[obj]
					}
					if candidates[dep] != "" {
						deps[obj] = append(deps[obj], dep)
					}
				}
				return true
			})
		}
	}
	for changed := true; changed; {
		changed = false
		for obj, targets := range deps {
			if needed[obj] != "" {
				continue
			}
			for _, dep := range targets {
				if needed[dep] != "" {
					needed[obj] = candidates[obj]
					changed = true
					break
				}
			}
		}
	}
	return needed
}

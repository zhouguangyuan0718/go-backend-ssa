package port

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"path"
	"strings"
)

// Literal bindings preserve the native value initializer and pass its result to
// an explicit instance binder. Only reviewed, explicit struct values are allowed;
// pointer allocation and implicit element literals need separate review.
func literalBindings(files []*ast.File, info *types.Info, units []Unit) (map[ast.Node]string, error) {
	result := map[ast.Node]string{}
	for i, u := range units {
		for _, rule := range u.Literals {
			count := 0
			var failure error
			ast.Inspect(files[i], func(n ast.Node) bool {
				lit, ok := n.(*ast.CompositeLit)
				if !ok {
					return true
				}
				typ := types.Unalias(info.TypeOf(lit))
				_, pointer := typ.(*types.Pointer)
				if pointer {
					typ = types.Unalias(typ.(*types.Pointer).Elem())
				}
				named, ok := typ.(*types.Named)
				if !ok || named.Obj().Pkg() == nil || named.Obj().Pkg().Path() != strings.TrimPrefix(path.Dir(rule.Source), "src/") || named.Obj().Name() != rule.Type {
					return true
				}
				if _, ok := named.Underlying().(*types.Struct); !ok || named.TypeParams().Len() != 0 || lit.Type == nil || pointer {
					failure = fmt.Errorf("literal binding requires an explicit non-generic struct value: %s", rule.Type)
					return false
				}
				if old := result[lit]; old != "" {
					failure = fmt.Errorf("duplicate literal binding: %s", rule.Type)
					return false
				}
				result[lit] = rule.Method
				count++
				return true
			})
			if failure != nil {
				return nil, failure
			}
			if count == 0 {
				return nil, fmt.Errorf("literal binding not used: %s", rule.Type)
			}
		}
	}
	for _, f := range files {
		var failure error
		ast.Inspect(f, func(n ast.Node) bool {
			if e, ok := n.(*ast.UnaryExpr); ok && e.Op == token.AND && result[e.X] != "" {
				failure = fmt.Errorf("literal binding requires a value, not an address")
			}
			return true
		})
		if failure != nil {
			return nil, failure
		}
	}
	return result, nil
}

func hasBoundLiteral(node ast.Node, bindings map[ast.Node]string) bool {
	found := false
	ast.Inspect(node, func(n ast.Node) bool { found = found || bindings[n] != ""; return !found })
	return found
}

func checkLiteralAdapters(units []Unit, before *packageImporter, out *types.Package, imports map[string]string) error {
	for _, u := range units {
		for _, rule := range u.Literals {
			pkg := before.packages[strings.TrimPrefix(path.Dir(rule.Source), "src/")]
			if pkg == nil {
				return fmt.Errorf("literal source package not selected: %s", rule.Source)
			}
			original, ok := pkg.Scope().Lookup(rule.Type).(*types.TypeName)
			if !ok || before.sources[original] != rule.Source {
				return fmt.Errorf("literal type source mismatch: %s", rule.Type)
			}
			ctx := out.Scope().Lookup(u.Context.Type)
			adapter, _, _ := lookupAdapterMember(types.NewPointer(ctx.Type()), out, rule.Method)
			fn, ok := adapter.(*types.Func)
			if !ok {
				return fmt.Errorf("missing literal adapter method %s", rule.Method)
			}
			sig := fn.Signature()
			if sig.Variadic() || sig.TypeParams().Len() != 0 || sig.Params().Len() != 1 || sig.Results().Len() != 1 {
				return fmt.Errorf("literal adapter must have signature func(T) T: %s", rule.Method)
			}
			want := types.TypeString(original.Type(), func(p *types.Package) string {
				if mapped := imports[p.Path()]; mapped != "" {
					return mapped
				}
				return p.Path()
			})
			for _, t := range []types.Type{sig.Params().At(0).Type(), sig.Results().At(0).Type()} {
				if types.TypeString(t, nil) != want {
					return fmt.Errorf("literal adapter type mismatch for %s: want %s", rule.Method, want)
				}
			}
		}
	}
	return nil
}

package port

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
)

// FixedSizeofRule admits one native sizeof observation of an empty composite
// literal in a reviewed function. It must be independent of the host layout.
type FixedSizeofRule struct{ Function, Type string }

func reviewedFixedSizeof(fs *token.FileSet, files []*ast.File, info *types.Info, pkg *types.Package, units []Unit) (map[ast.Node]bool, error) {
	allowed := map[ast.Node]bool{}
	for _, u := range units {
		for _, rule := range u.FixedSizeof {
			obj, ok := pkg.Scope().Lookup(rule.Type).(*types.TypeName)
			if !ok || !fixedSizeType(obj.Type()) {
				return nil, fmt.Errorf("sizeof type is not fixed-layout: %s", rule.Type)
			}
			var size int64 = -1
			for _, arch := range []string{"386", "amd64", "arm", "arm64", "ppc64", "ppc64le", "mips", "mipsle", "mips64", "mips64le", "loong64", "riscv64", "s390x", "wasm"} {
				layout := types.SizesFor("gc", arch)
				if layout == nil {
					return nil, fmt.Errorf("missing layout for %s", arch)
				}
				n := layout.Sizeof(obj.Type())
				if n < 0 || size >= 0 && n != size {
					return nil, fmt.Errorf("sizeof type varies by host: %s", rule.Type)
				}
				size = n
			}
			count := 0
			for _, f := range files {
				if fs.Position(f.Pos()).Filename != u.output() {
					continue
				}
				for _, d := range f.Decls {
					fn, ok := d.(*ast.FuncDecl)
					if !ok {
						continue
					}
					name := fn.Name.Name
					if fn.Recv != nil {
						name = receiverType(fn) + "." + name
					}
					if name != rule.Function {
						continue
					}
					ast.Inspect(fn.Body, func(n ast.Node) bool {
						call, ok := n.(*ast.CallExpr)
						if !ok || len(call.Args) != 1 {
							return true
						}
						sel, ok := call.Fun.(*ast.SelectorExpr)
						if !ok {
							return true
						}
						callee := info.Uses[sel.Sel]
						if callee == nil || callee.Pkg() == nil || callee.Pkg().Path() != "unsafe" || callee.Name() != "Sizeof" {
							return true
						}
						lit, ok := call.Args[0].(*ast.CompositeLit)
						if !ok || len(lit.Elts) != 0 || !types.Identical(info.TypeOf(lit), obj.Type()) {
							return true
						}
						allowed[sel.Sel] = true
						count++
						return true
					})
				}
			}
			if count != 1 {
				return nil, fmt.Errorf("reviewed sizeof use changed: %s (%d uses)", rule.Function, count)
			}
		}
	}
	return allowed, nil
}
func fixedSizeType(t types.Type) bool {
	switch t := t.Underlying().(type) {
	case *types.Basic:
		switch t.Kind() {
		case types.Bool, types.Int8, types.Int16, types.Int32, types.Int64, types.Uint8, types.Uint16, types.Uint32, types.Uint64, types.Float32, types.Float64, types.Complex64, types.Complex128:
			return true
		}
	case *types.Array:
		return fixedSizeType(t.Elem())
	case *types.Struct:
		for i := 0; i < t.NumFields(); i++ {
			if !fixedSizeType(t.Field(i).Type()) {
				return false
			}
		}
		return true
	}
	return false
}

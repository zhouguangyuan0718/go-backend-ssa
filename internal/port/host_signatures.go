package port

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/types"
)

func hostStreamMethod(name string) bool {
	switch name {
	case "Read", "Write", "Seek", "Close", "ReadAt", "Name":
		return true
	}
	return false
}

func hostStreamInterface(t types.Type, names []string) (*ast.InterfaceType, error) {
	ptr, ok := t.(*types.Pointer)
	if !ok {
		return nil, fmt.Errorf("host stream must be *os.File")
	}
	named, ok := ptr.Elem().(*types.Named)
	if !ok || named.Obj().Pkg() == nil || named.Obj().Pkg().Path() != "os" || named.Obj().Name() != "File" {
		return nil, fmt.Errorf("host stream must be *os.File")
	}
	methods := &ast.FieldList{}
	for _, name := range names {
		obj, index, _ := types.LookupFieldOrMethod(ptr, true, nil, name)
		fn, ok := obj.(*types.Func)
		if !hostStreamMethod(name) || !ok || len(index) != 1 {
			return nil, fmt.Errorf("host stream method not available: %s", name)
		}
		typ, err := parser.ParseExpr(types.TypeString(unnamedSignature(fn.Type()), func(p *types.Package) string { return p.Path() }))
		if err != nil {
			return nil, err
		}
		foreign := false
		ast.Inspect(typ, func(n ast.Node) bool {
			if _, ok := n.(*ast.SelectorExpr); ok {
				foreign = true
			}
			return true
		})
		if foreign {
			return nil, fmt.Errorf("host stream method has a nonportable signature: %s", name)
		}
		methods.List = append(methods.List, &ast.Field{Names: []*ast.Ident{ast.NewIdent(name)}, Type: typ})
	}
	return &ast.InterfaceType{Methods: methods}, nil
}

func rewriteHostSignatures(files []*ast.File, info *types.Info, units []Unit) error {
	for i, u := range units {
		for _, rule := range u.HostSignatures {
			found := false
			for _, decl := range files[i].Decls {
				fn, ok := decl.(*ast.FuncDecl)
				if !ok {
					continue
				}
				obj, ok := info.Defs[fn.Name].(*types.Func)
				if !ok {
					continue
				}
				name := obj.Name()
				sig := obj.Type().(*types.Signature)
				if sig.Recv() != nil {
					t := sig.Recv().Type()
					if p, ok := t.(*types.Pointer); ok {
						t = p.Elem()
					}
					n, ok := t.(*types.Named)
					if !ok {
						return fmt.Errorf("host stream receiver is not named")
					}
					name = n.Obj().Name() + "." + name
				}
				if name != rule.Function {
					continue
				}
				list := fn.Type.Params
				if rule.Result {
					list = fn.Type.Results
				}
				index := 0
				if list != nil {
					for _, field := range list.List {
						count := len(field.Names)
						if count == 0 {
							count = 1
						}
						if rule.Index >= index && rule.Index < index+count {
							// Do not silently widen siblings in a grouped parameter declaration.
							if count != 1 {
								return fmt.Errorf("host stream signature slot must be ungrouped: %s", name)
							}
							typ, err := hostStreamInterface(info.TypeOf(field.Type), rule.Methods)
							if err != nil {
								return fmt.Errorf("host stream signature %s: %w", name, err)
							}
							field.Type = typ
							found = true
							break
						}
						index += count
					}
				}
			}
			if !found {
				return fmt.Errorf("host stream signature slot not selected: %s:%t:%d", rule.Function, rule.Result, rule.Index)
			}
		}
	}
	return nil
}

package port

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"reflect"
)

// SliceReinterpretationRule reviews a paired native scratch-slice allocator.
// Both methods must keep the checked header construction and capacity arithmetic;
// it does not grant a function or file general permission to use unsafe.
type SliceReinterpretationRule struct {
	Allocate, Free         string
	BaseAllocate, BaseFree string
}

func reviewedSliceOperations(fs *token.FileSet, files []*ast.File, info *types.Info, units []Unit) (map[ast.Node]bool, error) {
	allowed := map[ast.Node]bool{}
	for _, u := range units {
		for _, rule := range u.SliceReinterpretations {
			methods := map[string]*ast.FuncDecl{}
			for _, f := range files {
				for _, d := range f.Decls {
					if fn, ok := d.(*ast.FuncDecl); ok && fn.Recv != nil && fs.Position(fn.Pos()).Filename == u.output() {
						methods[receiverType(fn)+"."+fn.Name.Name] = fn
					}
				}
			}
			alloc, free := methods[rule.Allocate], methods[rule.Free]
			if alloc == nil || free == nil {
				return nil, fmt.Errorf("slice reinterpretation methods missing: %s / %s", rule.Allocate, rule.Free)
			}
			a := info.Defs[alloc.Name].Type().(*types.Signature)
			f := info.Defs[free.Name].Type().(*types.Signature)
			if a.Variadic() || f.Variadic() || a.Params().Len() != 1 || a.Results().Len() != 1 || f.Params().Len() != 1 || f.Results().Len() != 0 || !types.Identical(a.Params().At(0).Type(), types.Typ[types.Int]) || !types.Identical(a.Recv().Type(), f.Recv().Type()) {
				return nil, fmt.Errorf("invalid slice reinterpretation signature: %s", rule.Allocate)
			}
			derived, ok := a.Results().At(0).Type().(*types.Slice)
			if !ok || !types.Identical(f.Params().At(0).Type(), derived) {
				return nil, fmt.Errorf("slice reinterpretation pair disagrees: %s", rule.Allocate)
			}
			baseAlloc := lookupSliceMethod(a.Recv().Type(), rule.BaseAllocate, info.Defs[alloc.Name].Pkg())
			baseFree := lookupSliceMethod(a.Recv().Type(), rule.BaseFree, info.Defs[alloc.Name].Pkg())
			if baseAlloc == nil || baseFree == nil {
				return nil, fmt.Errorf("slice reinterpretation base methods missing: %s", rule.Allocate)
			}
			ba, bf := baseAlloc.Type().(*types.Signature), baseFree.Type().(*types.Signature)
			if ba.Variadic() || bf.Variadic() || ba.Params().Len() != 1 || ba.Results().Len() != 1 || bf.Params().Len() != 1 || bf.Results().Len() != 0 || !types.Identical(ba.Params().At(0).Type(), types.Typ[types.Int]) {
				return nil, fmt.Errorf("invalid base slice allocator signature: %s", rule.Allocate)
			}
			base, ok := ba.Results().At(0).Type().(*types.Slice)
			if !ok || !types.Identical(bf.Params().At(0).Type(), base) {
				return nil, fmt.Errorf("base slice allocator pair disagrees: %s", rule.Allocate)
			}
			if err := checkSliceElementLayouts(base.Elem(), derived.Elem()); err != nil {
				return nil, fmt.Errorf("%s: %w", rule.Allocate, err)
			}
			for _, item := range []struct {
				fn    *ast.FuncDecl
				sig   *types.Signature
				body  string
				param string
			}{
				{alloc, a, sliceAllocatePattern(), "n"}, {free, f, sliceFreePattern(), "s"},
			} {
				pattern, err := parser.ParseFile(token.NewFileSet(), "pattern.go", "package p;func pattern(){"+item.body+"}", 0)
				if err != nil {
					panic(err)
				}
				matcher := slicePatternMatcher{info: info, objects: map[string]types.Object{"c": item.sig.Recv(), item.param: item.sig.Params().At(0)}, types: map[string]types.Type{"baseType": base.Elem(), "derivedType": derived.Elem()}, methods: map[string]*types.Func{"baseAllocate": baseAlloc, "baseFree": baseFree}}
				// The one local header retains a GC-visible unsafe.Pointer, followed by
				// native int length/capacity fields. Integer-address headers are rejected.
				headers := 0
				ast.Inspect(item.fn.Body, func(n ast.Node) bool {
					if lit, ok := n.(*ast.CompositeLit); ok {
						headers++
						matcher.types["headerType"] = info.TypeOf(lit.Type)
					}
					return true
				})
				if headers != 1 || !nativeSliceHeader(matcher.types["headerType"]) {
					return nil, fmt.Errorf("invalid GC-visible slice header in %s", item.fn.Name.Name)
				}
				if !matcher.match(reflect.ValueOf(item.fn.Body), reflect.ValueOf(pattern.Decls[0].(*ast.FuncDecl).Body)) {
					return nil, fmt.Errorf("native slice reinterpretation pattern changed: %s", item.fn.Name.Name)
				}
				ast.Inspect(item.fn.Body, func(n ast.Node) bool {
					if e, ok := n.(ast.Expr); ok {
						if t, ok := info.TypeOf(e).(*types.Basic); ok && t.Kind() == types.UnsafePointer {
							allowed[n] = true
						}
					}
					if id, ok := n.(*ast.Ident); ok {
						if obj := info.Uses[id]; obj != nil && obj.Pkg() != nil && obj.Pkg().Path() == "unsafe" {
							allowed[id] = true
						}
					}
					return true
				})
			}
		}
	}
	return allowed, nil
}

func lookupSliceMethod(receiver types.Type, name string, pkg *types.Package) *types.Func {
	owner, method, ok := splitMethod(name)
	if !ok {
		return nil
	}
	ptr, ok := receiver.(*types.Pointer)
	if !ok {
		return nil
	}
	named, ok := ptr.Elem().(*types.Named)
	if !ok || named.Obj().Name() != owner {
		return nil
	}
	obj, _, _ := types.LookupFieldOrMethod(receiver, true, pkg, method)
	fn, _ := obj.(*types.Func)
	return fn
}

func splitMethod(name string) (string, string, bool) {
	for i, c := range name {
		if c == '.' {
			return name[:i], name[i+1:], true
		}
	}
	return "", "", false
}

func nativeSliceHeader(t types.Type) bool {
	if t == nil {
		return false
	}
	s, ok := t.Underlying().(*types.Struct)
	if !ok || s.NumFields() != 3 {
		return false
	}
	for i, name := range []string{"Data", "Len", "Cap"} {
		f := s.Field(i)
		want := types.Typ[types.Int]
		if i == 0 {
			want = types.Typ[types.UnsafePointer]
		}
		if f.Name() != name || f.Embedded() || !types.Identical(f.Type(), want) {
			return false
		}
	}
	return true
}

func checkSliceElementLayouts(base, derived types.Type) error {
	_, bp := base.Underlying().(*types.Pointer)
	_, dp := derived.Underlying().(*types.Pointer)
	if !(bp && dp) && !(pointerFreeSliceElement(base) && pointerFreeSliceElement(derived)) {
		return fmt.Errorf("slice element GC layouts differ")
	}
	// Check every native host layout supported by gc, not just the generator's
	// current pointer width. The native sizeof expressions remain in the output.
	for _, arch := range []string{"386", "amd64", "arm", "arm64", "loong64", "mips", "mips64", "ppc64", "riscv64", "s390x", "wasm"} {
		sizes := types.SizesFor("gc", arch)
		b, d := sizes.Sizeof(base), sizes.Sizeof(derived)
		if b <= 0 || d <= 0 || b%d != 0 || sizes.Alignof(base) < sizes.Alignof(derived) {
			return fmt.Errorf("incompatible scratch element layout on %s", arch)
		}
	}
	return nil
}

func pointerFreeSliceElement(t types.Type) bool {
	switch t := t.Underlying().(type) {
	case *types.Basic:
		return t.Info()&(types.IsBoolean|types.IsInteger|types.IsFloat|types.IsComplex) != 0
	case *types.Array:
		return pointerFreeSliceElement(t.Elem())
	case *types.Struct:
		for i := 0; i < t.NumFields(); i++ {
			if !pointerFreeSliceElement(t.Field(i).Type()) {
				return false
			}
		}
		return true
	}
	return false
}

type slicePatternMatcher struct {
	info    *types.Info
	objects map[string]types.Object
	types   map[string]types.Type
	methods map[string]*types.Func
}

// Match AST shape while identifying locals, builtins, fields and imported
// operations by their checked objects. Renaming local variables/import aliases
// is harmless; shadowed builtins or different length/capacity arithmetic is not.
func (m *slicePatternMatcher) match(a, p reflect.Value) bool {
	if !a.IsValid() || !p.IsValid() {
		return a.IsValid() == p.IsValid()
	}
	if p.Kind() == reflect.Interface {
		if p.IsNil() {
			return a.Kind() == reflect.Interface && a.IsNil()
		}
		if a.Kind() != reflect.Interface || a.IsNil() {
			return false
		}
		return m.match(a.Elem(), p.Elem())
	}
	if p.CanInterface() {
		if id, ok := p.Interface().(*ast.Ident); ok && id != nil {
			if typ := m.types[id.Name]; typ != nil {
				expr, ok := a.Interface().(ast.Expr)
				return ok && types.Identical(m.info.TypeOf(expr), typ)
			}
			actual, ok := a.Interface().(*ast.Ident)
			if !ok || actual == nil {
				return false
			}
			obj := m.info.Uses[actual]
			if obj == nil {
				obj = m.info.Defs[actual]
			}
			if builtin := types.Universe.Lookup(id.Name); builtin != nil {
				return obj == builtin
			}
			if id.Name == "Data" || id.Name == "Len" || id.Name == "Cap" {
				st := m.types["headerType"].Underlying().(*types.Struct)
				for i := 0; i < 3; i++ {
					if st.Field(i).Name() == id.Name {
						return obj == st.Field(i)
					}
				}
			}
			if previous := m.objects[id.Name]; previous != nil {
				return obj == previous
			}
			if v, ok := m.info.Defs[actual].(*types.Var); ok && v.Parent() != v.Pkg().Scope() {
				m.objects[id.Name] = v
				return true
			}
			return false
		}
		if sel, ok := p.Interface().(*ast.SelectorExpr); ok && sel != nil {
			if pkg, ok := sel.X.(*ast.Ident); ok && pkg.Name == "unsafe" {
				var obj types.Object
				switch expr := a.Interface().(type) {
				case *ast.SelectorExpr:
					obj = m.info.Uses[expr.Sel]
				case *ast.Ident:
					obj = m.info.Uses[expr]
				}
				return obj != nil && obj.Pkg() != nil && obj.Pkg().Path() == "unsafe" && obj.Name() == sel.Sel.Name
			}
			if method := m.methods[sel.Sel.Name]; method != nil {
				actual, ok := a.Interface().(*ast.SelectorExpr)
				return ok && m.info.Uses[actual.Sel] == method && m.match(reflect.ValueOf(actual.X), reflect.ValueOf(sel.X))
			}
		}
	}
	if a.Type() != p.Type() {
		return false
	}
	if p.Type() == reflect.TypeFor[token.Pos]() {
		return true
	}
	switch p.Kind() {
	case reflect.Pointer:
		if a.IsNil() || p.IsNil() {
			return a.IsNil() == p.IsNil()
		}
		return m.match(a.Elem(), p.Elem())
	case reflect.Struct:
		for i := 0; i < p.NumField(); i++ {
			name := p.Type().Field(i).Name
			if name == "Obj" || name == "Scope" || name == "Doc" || name == "Comment" || name == "Comments" {
				continue
			}
			if !m.match(a.Field(i), p.Field(i)) {
				return false
			}
		}
		return true
	case reflect.Slice:
		if a.Len() != p.Len() {
			return false
		}
		for i := 0; i < p.Len(); i++ {
			if !m.match(a.Index(i), p.Index(i)) {
				return false
			}
		}
		return true
	case reflect.String:
		return a.String() == p.String()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return a.Int() == p.Int()
	case reflect.Bool:
		return a.Bool() == p.Bool()
	}
	return false
}

// These are shape constraints for the upstream generated algorithms, not an
// emitted implementation. The original AST, comments and method bodies survive.
func sliceAllocatePattern() string {
	return `
 var base baseType
 var derived derivedType
 if unsafe.Sizeof(base)%unsafe.Sizeof(derived)!=0{panic("bad")}
 scale:=unsafe.Sizeof(base)/unsafe.Sizeof(derived)
 b:=c.baseAllocate(int((uintptr(n)+scale-1)/scale))
 s:=headerType{Data:unsafe.Pointer(&b[0]),Len:n,Cap:cap(b)*int(scale)}
 return *(*[]derivedType)(unsafe.Pointer(&s))
`
}
func sliceFreePattern() string {
	return `
 var base baseType
 var derived derivedType
 scale:=unsafe.Sizeof(base)/unsafe.Sizeof(derived)
 b:=headerType{Data:unsafe.Pointer(&s[0]),Len:int((uintptr(len(s))+scale-1)/scale),Cap:int((uintptr(cap(s))+scale-1)/scale)}
 c.baseFree(*(*[]baseType)(unsafe.Pointer(&b)))
`
}

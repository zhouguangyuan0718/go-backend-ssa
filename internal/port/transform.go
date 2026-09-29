package port

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"path"
	"reflect"
	"sort"
	"strconv"
	"strings"
)

func nodeText(fset *token.FileSet, n ast.Node) string {
	var b bytes.Buffer
	_ = format.Node(&b, fset, n)
	return b.String()
}

type packageImporter struct {
	packages map[string]*types.Package
	standard types.Importer
}

func newImporter() *packageImporter {
	return &packageImporter{map[string]*types.Package{}, importer.Default()}
}
func (i *packageImporter) Import(p string) (*types.Package, error) {
	if pkg := i.packages[p]; pkg != nil {
		return pkg, nil
	}
	if strings.HasPrefix(p, "cmd/") || strings.HasPrefix(p, "internal/") || strings.HasPrefix(p, "go-backend-ssa/") {
		return nil, fmt.Errorf("unresolved internal import %s; add an explicit prior unit or adapter", p)
	}
	return i.standard.Import(p)
}

func checkTypes(fset *token.FileSet, pkg string, files []*ast.File, imp types.Importer) (*types.Package, *types.Info, error) {
	info := &types.Info{Defs: map[*ast.Ident]types.Object{}, Uses: map[*ast.Ident]types.Object{}, Types: map[ast.Expr]types.TypeAndValue{}}
	c := types.Config{Importer: imp, GoVersion: "go1.27"}
	p, err := c.Check(pkg, fset, files, info)
	return p, info, err
}

// selectFile selects top-level functions and single-name state declarations.
// Missing dependencies fail the type check; no stubs are supplied.
func selectFile(fset *token.FileSet, name string, data []byte, u Unit) (*ast.File, error) {
	f, err := parser.ParseFile(fset, name, data, parser.ParseComments|parser.SkipObjectResolution)
	if err != nil {
		return nil, err
	}
	want := map[string]string{}
	for _, fn := range u.Functions {
		if want[fn] != "" {
			return nil, fmt.Errorf("duplicate function %s", fn)
		}
		want[fn] = "func"
	}
	if u.Context != nil {
		for state := range u.Context.Fields {
			if want[state] != "" {
				return nil, fmt.Errorf("duplicate state %s", state)
			}
			want[state] = "var"
		}
	}
	found := map[string]bool{}
	var decls []ast.Decl
	for _, d := range f.Decls {
		switch d := d.(type) {
		case *ast.FuncDecl:
			if u.Context != nil && d.Name.Name == "init" {
				return nil, fmt.Errorf("package init requires an explicit adaptation rule: %s", name)
			}
			if want[d.Name.Name] != "func" {
				continue
			}
			if d.Recv != nil || d.Type.TypeParams != nil || d.Body == nil || d.Name.Name == "init" {
				return nil, fmt.Errorf("unsupported function shape: %s", d.Name.Name)
			}
			if found[d.Name.Name] {
				return nil, fmt.Errorf("ambiguous function: %s", d.Name.Name)
			}
			found[d.Name.Name] = true
			decls = append(decls, d)
		case *ast.GenDecl:
			if d.Tok == token.IMPORT {
				decls = append(decls, d)
				continue
			}
			var specs []ast.Spec
			for _, sp := range d.Specs {
				v, ok := sp.(*ast.ValueSpec)
				if !ok {
					continue
				}
				for _, id := range v.Names {
					if want[id.Name] == "var" {
						if d.Tok != token.VAR || len(v.Names) != 1 || len(v.Values) > 1 {
							return nil, fmt.Errorf("unsupported state declaration: %s", id.Name)
						}
						found[id.Name] = true
						specs = append(specs, v)
					}
				}
			}
			if len(specs) > 0 {
				d.Specs = specs
				decls = append(decls, d)
			}
		}
	}
	for name := range want {
		if !found[name] {
			return nil, fmt.Errorf("selected declaration not found: %s", name)
		}
	}
	comments := ast.NewCommentMap(fset, f, f.Comments)
	f.Decls = decls
	f.Comments = comments.Filter(f).Comments()
	pruneImports(f)
	return f, nil
}

func pruneImports(f *ast.File, extra ...ast.Node) {
	used := map[string]bool{}
	visit := func(n ast.Node) bool {
		if s, ok := n.(*ast.SelectorExpr); ok {
			if id, ok := s.X.(*ast.Ident); ok {
				used[id.Name] = true
			}
		}
		return true
	}
	ast.Inspect(f, visit)
	for _, n := range extra {
		ast.Inspect(n, visit)
	}
	var decls []ast.Decl
	f.Imports = nil
	for _, d := range f.Decls {
		g, ok := d.(*ast.GenDecl)
		if !ok || g.Tok != token.IMPORT {
			decls = append(decls, d)
			continue
		}
		var specs []ast.Spec
		for _, sp := range g.Specs {
			im := sp.(*ast.ImportSpec)
			p, _ := strconv.Unquote(im.Path.Value)
			alias := path.Base(p)
			if im.Name != nil {
				alias = im.Name.Name
			}
			if used[alias] || alias == "." || alias == "_" {
				specs = append(specs, im)
				f.Imports = append(f.Imports, im)
			}
		}
		if len(specs) > 0 {
			g.Specs = specs
			decls = append(decls, g)
		}
	}
	f.Decls = decls
}

// rewriteNodes walks AST fields only. It is not a textual identifier rewrite.
func rewriteNodes(node ast.Node, replace map[*ast.Ident]ast.Expr) {
	nodeType := reflect.TypeFor[ast.Node]()
	var walk func(reflect.Value)
	walk = func(v reflect.Value) {
		if !v.IsValid() {
			return
		}
		if v.Kind() == reflect.Interface {
			if v.IsNil() {
				return
			}
			if id, ok := v.Interface().(*ast.Ident); ok {
				if r := replace[id]; r != nil {
					v.Set(reflect.ValueOf(r))
					return
				}
			}
			walk(v.Elem())
			return
		}
		if v.Kind() == reflect.Pointer {
			if v.IsNil() || !v.Type().Implements(nodeType) {
				return
			}
			walk(v.Elem())
			return
		}
		if v.Kind() == reflect.Struct {
			for i := 0; i < v.NumField(); i++ {
				walk(v.Field(i))
			}
		}
		if v.Kind() == reflect.Slice {
			for i := 0; i < v.Len(); i++ {
				walk(v.Index(i))
			}
		}
	}
	walk(reflect.ValueOf(node))
}

func transform(fset *token.FileSet, f *ast.File, info *types.Info, u Unit, mapping map[string]string) ([]byte, error) {
	var constructor string
	if c := u.Context; c != nil {
		for _, name := range []string{c.Type, c.Constructor} {
			if !token.IsIdentifier(name) || token.Lookup(name).IsKeyword() {
				return nil, fmt.Errorf("invalid context identifier %s", name)
			}
		}
		fields := map[string]bool{}
		for _, name := range c.Fields {
			if !token.IsIdentifier(name) || fields[name] {
				return nil, fmt.Errorf("invalid or duplicate context field %s", name)
			}
			fields[name] = true
		}
		const recv = "syncgoCtx"
		collision := false
		ast.Inspect(f, func(n ast.Node) bool {
			if id, ok := n.(*ast.Ident); ok && (id.Name == recv || id.Name == c.Constructor || id.Name == c.Type) {
				collision = true
			}
			return true
		})
		if collision {
			return nil, fmt.Errorf("context identifier collision in %s", u.Source)
		}
		states := map[types.Object]string{}
		functions := map[types.Object]string{}
		for id, obj := range info.Defs {
			if field, ok := c.Fields[id.Name]; ok && obj != nil && obj.Parent() == obj.Pkg().Scope() {
				states[obj] = field
			}
		}
		for _, d := range f.Decls {
			if fn, ok := d.(*ast.FuncDecl); ok {
				functions[info.Defs[fn.Name]] = fn.Name.Name
			}
		}
		replacements := map[*ast.Ident]ast.Expr{}
		for id, obj := range info.Uses {
			name := states[obj]
			if name == "" {
				name = functions[obj]
			}
			if name != "" {
				replacements[id] = &ast.SelectorExpr{X: &ast.Ident{Name: recv, NamePos: id.Pos()}, Sel: &ast.Ident{Name: name, NamePos: id.Pos()}}
			}
		}
		rewriteNodes(f, replacements)
		var init strings.Builder
		fmt.Fprintf(&init, "\nfunc %s() *%s {\n%s := &%s{}\n", c.Constructor, c.Type, recv, c.Type)
		for _, entry := range info.InitOrder {
			if len(entry.Lhs) != 1 || states[entry.Lhs[0]] == "" {
				return nil, fmt.Errorf("unsupported state initialization")
			}
			// InitOrder can retain an old root identifier after the ValueSpec's
			// expression slot has been replaced. Rewrite that slot explicitly.
			expr := &ast.ParenExpr{X: entry.Rhs}
			rewriteNodes(expr, replacements)
			fmt.Fprintf(&init, "%s.%s = %s\n", recv, states[entry.Lhs[0]], nodeText(fset, expr.X))
		}
		fmt.Fprintf(&init, "return %s\n}\n", recv)
		constructor = init.String()
		var decls []ast.Decl
		for _, d := range f.Decls {
			if g, ok := d.(*ast.GenDecl); ok && g.Tok == token.VAR {
				continue
			}
			if fn, ok := d.(*ast.FuncDecl); ok {
				fn.Recv = &ast.FieldList{List: []*ast.Field{{Names: []*ast.Ident{ast.NewIdent(recv)}, Type: &ast.StarExpr{X: ast.NewIdent(c.Type)}}}}
			}
			decls = append(decls, d)
		}
		cm := ast.NewCommentMap(fset, f, f.Comments)
		f.Decls = decls
		f.Comments = cm.Filter(f).Comments()
	}
	if constructor != "" {
		cf, err := parser.ParseFile(token.NewFileSet(), "constructor.go", "package p\n"+constructor, 0)
		if err != nil {
			return nil, err
		}
		pruneImports(f, cf)
	} else {
		pruneImports(f)
	}
	for _, im := range f.Imports {
		p, _ := strconv.Unquote(im.Path.Value)
		if im.Name != nil && (im.Name.Name == "." || im.Name.Name == "_") {
			return nil, fmt.Errorf("unsupported dot/side-effect import %s", p)
		}
		if dst := mapping[p]; dst != "" {
			if im.Name == nil && path.Base(dst) != path.Base(p) {
				im.Name = ast.NewIdent(path.Base(p))
			}
			im.Path.Value = strconv.Quote(dst)
		} else if strings.HasPrefix(p, "cmd/") || strings.HasPrefix(p, "internal/") {
			return nil, fmt.Errorf("residual upstream internal import %s", p)
		}
	}
	var out bytes.Buffer
	fmt.Fprintf(&out, "// Code generated by %s from %s; DO NOT EDIT.\n", ToolVersion, u.Source)
	if err := format.Node(&out, fset, f); err != nil {
		return nil, err
	}
	out.WriteString(constructor)
	return format.Source(out.Bytes())
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

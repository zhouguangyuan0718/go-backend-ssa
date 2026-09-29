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

// selectFile either retains a whole file or selects the supported function/state
// slice. Missing dependencies fail package type checking; no stubs are supplied.
func selectFile(fset *token.FileSet, name string, data []byte, u Unit) (*ast.File, error) {
	f, err := parser.ParseFile(fset, name, data, parser.ParseComments|parser.SkipObjectResolution)
	if err != nil {
		return nil, err
	}
	if u.WholeFile {
		for _, d := range f.Decls {
			if fn, ok := d.(*ast.FuncDecl); ok && fn.Recv == nil && fn.Name.Name == "init" {
				return nil, fmt.Errorf("package init requires an explicit adaptation rule: %s", name)
			}
		}
		return f, nil
	}
	want := map[string]string{}
	for _, fn := range u.Functions {
		if want[fn] != "" {
			return nil, fmt.Errorf("duplicate function %s", fn)
		}
		want[fn] = "func"
	}
	for _, name := range u.Declarations {
		if want[name] != "" {
			return nil, fmt.Errorf("duplicate declaration %s", name)
		}
		want[name] = "declaration"
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
			if u.Context != nil && d.Recv == nil && d.Name.Name == "init" {
				if !u.Context.Init {
					return nil, fmt.Errorf("package init requires an explicit adaptation rule: %s", name)
				}
				decls = append(decls, d)
				continue
			}
			key := d.Name.Name
			if d.Recv != nil {
				key = receiverType(d) + "." + key
			}
			if want[key] != "func" && want[key] != "declaration" {
				continue
			}
			if (d.Recv != nil && want[key] != "declaration") || d.Type.TypeParams != nil || d.Body == nil || (d.Recv == nil && d.Name.Name == "init") {
				return nil, fmt.Errorf("unsupported function shape: %s", d.Name.Name)
			}
			if found[key] {
				return nil, fmt.Errorf("ambiguous function: %s", d.Name.Name)
			}
			found[key] = true
			decls = append(decls, d)
		case *ast.GenDecl:
			if d.Tok == token.IMPORT {
				decls = append(decls, d)
				continue
			}
			// Keep an entire constant group so omitted expressions and iota
			// retain their original values. A selected member anchors the group.
			if d.Tok == token.CONST {
				keep := false
				for _, sp := range d.Specs {
					for _, id := range sp.(*ast.ValueSpec).Names {
						if want[id.Name] == "declaration" {
							found[id.Name] = true
							keep = true
						}
					}
				}
				if keep {
					decls = append(decls, d)
				}
				continue
			}
			var specs []ast.Spec
			for _, sp := range d.Specs {
				if typ, ok := sp.(*ast.TypeSpec); ok {
					if want[typ.Name.Name] == "declaration" {
						found[typ.Name.Name] = true
						specs = append(specs, sp)
					}
					continue
				}
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
	pruneImportsWithInfo(f, nil, extra...)
}

func pruneImportsWithInfo(f *ast.File, info *types.Info, extra ...ast.Node) {
	used := map[string]bool{}
	visit := func(n ast.Node) bool {
		if s, ok := n.(*ast.SelectorExpr); ok {
			if id, ok := s.X.(*ast.Ident); ok {
				if info != nil {
					if obj, known := info.Uses[id]; known {
						if _, imported := obj.(*types.PkgName); !imported {
							return true
						}
					}
				}
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
	exprs := make(map[ast.Node]ast.Expr, len(replace))
	for id, expr := range replace {
		exprs[id] = expr
	}
	rewriteExprs(node, exprs)
}

func rewriteExprs(node ast.Node, replace map[ast.Node]ast.Expr) {
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
			if n, ok := v.Interface().(ast.Node); ok {
				if r := replace[n]; r != nil {
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
	if u.Owner != nil {
		if err := addOwnerFields(f, u.Owner); err != nil {
			return nil, err
		}
	}
	pruneImportsWithInfo(f, info)
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

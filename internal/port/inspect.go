package port

import (
	"crypto/sha256"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"path"
	"sort"
	"strconv"
	"strings"
)

type Inventory struct {
	SchemaVersion int             `json:"schema_version"`
	Commit        string          `json:"commit"`
	Tool          string          `json:"tool"`
	Scope         string          `json:"scope"`
	Limitations   []string        `json:"limitations"`
	Packages      []PackageRecord `json:"packages"`
}

type PackageRecord struct {
	Path         string         `json:"path"`
	Files        []FileRecord   `json:"files"`
	Imports      []ImportRecord `json:"imports"`
	Declarations []Declaration  `json:"declarations"`
	References   []Reference    `json:"references"`
	Hazards      []Hazard       `json:"hazards"`
}

type FileRecord struct {
	Path             string   `json:"path"`
	SHA256           string   `json:"sha256"`
	BuildConstraints []string `json:"build_constraints,omitempty"`
	Excluded         string   `json:"excluded,omitempty"`
}
type ImportRecord struct {
	Path     string `json:"path"`
	Resolved string `json:"resolved"`
	Boundary string `json:"boundary"`
}
type Declaration struct {
	ID                      string   `json:"id"`
	Kind                    string   `json:"kind"`
	File                    string   `json:"file"`
	Line                    int      `json:"line"`
	StateReview             string   `json:"state_review,omitempty"`
	Initializer             string   `json:"initializer,omitempty"`
	TypeSyntax              string   `json:"type_syntax,omitempty"`
	InitializerDependencies []string `json:"initializer_dependencies,omitempty"`
}
type Reference struct {
	From string `json:"from"`
	To   string `json:"to"`
	File string `json:"file"`
	Line int    `json:"line"`
	Use  string `json:"use"`
}
type Hazard struct {
	File   string `json:"file"`
	Line   int    `json:"line"`
	Kind   string `json:"kind"`
	Detail string `json:"detail"`
}

type InventorySummary struct {
	SchemaVersion     int              `json:"schema_version"`
	Commit            string           `json:"commit"`
	Tool              string           `json:"tool"`
	Scope             string           `json:"scope"`
	InventorySHA256   string           `json:"inventory_sha256"`
	PackageCount      int              `json:"package_count"`
	FileCount         int              `json:"file_count"`
	DeclarationCount  int              `json:"declaration_count"`
	VariableCount     int              `json:"variable_count"`
	ReferenceCount    int              `json:"reference_count"`
	HazardCount       int              `json:"hazard_count"`
	UnresolvedImports []string         `json:"unresolved_imports"`
	Packages          []PackageSummary `json:"packages"`
}
type PackageSummary struct {
	Path      string `json:"path"`
	Files     int    `json:"files"`
	Variables int    `json:"variables"`
	Hazards   int    `json:"hazards"`
}

func Summarize(r *Inventory, encoded []byte) InventorySummary {
	s := InventorySummary{SchemaVersion: 1, Commit: r.Commit, Tool: r.Tool, Scope: r.Scope, InventorySHA256: hash(encoded), PackageCount: len(r.Packages), UnresolvedImports: []string{}}
	for _, p := range r.Packages {
		ps := PackageSummary{Path: p.Path, Files: len(p.Files), Hazards: len(p.Hazards)}
		for _, d := range p.Declarations {
			if d.Kind == "var" {
				ps.Variables++
			}
		}
		for _, im := range p.Imports {
			if im.Boundary == "external-unresolved" {
				s.UnresolvedImports = append(s.UnresolvedImports, p.Path+" -> "+im.Path)
			}
		}
		s.FileCount += ps.Files
		s.VariableCount += ps.Variables
		s.HazardCount += ps.Hazards
		s.DeclarationCount += len(p.Declarations)
		s.ReferenceCount += len(p.References)
		s.Packages = append(s.Packages, ps)
	}
	return s
}

func hash(data []byte) string { return fmt.Sprintf("%x", sha256.Sum256(data)) }

// Inspect inventories all build variants, not just the host's selected files.
// Standard library APIs are boundaries; cmd/internal, internal and vendored
// implementation packages are recursively inspected. References are syntactic
// evidence, deliberately not a claim of complete alias or call-graph analysis.
func Inspect(s *Source, roots []string) (*Inventory, error) {
	r := &Inventory{SchemaVersion: 1, Commit: s.Pin.Commit, Tool: ToolVersion,
		Scope:       "union of production build variants in recursive compiler/internal/vendor import closure; standard library APIs are boundaries",
		Limitations: []string{"AST reference evidence, not alias analysis or a complete dynamic call graph", "all package variables require review, including zero-write maps, pointers, callbacks and lazy caches", "target filename suffixes and build directives are retained; mutually exclusive declarations may coexist", "test files and go:build ignore / package main tools are excluded", "unresolved dot imports and indirect mutation are explicitly flagged", "multi-name initializers conservatively share syntactic dependency evidence"}}
	queue := append([]string(nil), roots...)
	seen := map[string]bool{}
	for len(queue) > 0 {
		pkg := queue[0]
		queue = queue[1:]
		if seen[pkg] {
			continue
		}
		seen[pkg] = true
		files := s.PackageFiles(pkg)
		if len(files) == 0 {
			return nil, fmt.Errorf("no source files for package %s", pkg)
		}
		if err := s.Load(files); err != nil {
			return nil, err
		}
		pr, err := inspectPackage(s, pkg, files)
		if err != nil {
			return nil, err
		}
		for _, im := range pr.Imports {
			if im.Boundary == "inspect" {
				queue = append(queue, im.Resolved)
			}
		}
		r.Packages = append(r.Packages, *pr)
	}
	sort.Slice(r.Packages, func(i, j int) bool { return r.Packages[i].Path < r.Packages[j].Path })
	return r, nil
}

func resolveImport(s *Source, from, imp string) ImportRecord {
	r := ImportRecord{Path: imp, Resolved: imp, Boundary: "stdlib"}
	if strings.HasPrefix(imp, "cmd/") || strings.HasPrefix(imp, "internal/") {
		r.Boundary = "inspect"
		return r
	}
	for dir := "src/" + from; dir != "."; dir = path.Dir(dir) {
		candidate := strings.TrimPrefix(dir+"/vendor/"+imp, "src/")
		if len(s.PackageFiles(candidate)) > 0 {
			r.Resolved = candidate
			r.Boundary = "inspect"
			return r
		}
	}
	if strings.Contains(strings.Split(imp, "/")[0], ".") || imp == "C" {
		r.Boundary = "external-unresolved"
	}
	return r
}

func inspectPackage(s *Source, pkg string, names []string) (*PackageRecord, error) {
	r := &PackageRecord{Path: pkg}
	fset := token.NewFileSet()
	parsed := map[string]*ast.File{}
	owners := map[ast.Node][]string{}
	declIDs := map[*ast.Ident]bool{}
	objects := map[*ast.Object]string{}
	top := map[string]string{}
	imports := map[string]ImportRecord{}
	for _, name := range names {
		data := s.cache[name]
		f, err := parser.ParseFile(fset, name, data, parser.ParseComments)
		if err != nil {
			return nil, err
		}
		fr := FileRecord{Path: name, SHA256: hash(data)}
		for _, cg := range f.Comments {
			for _, c := range cg.List {
				if strings.HasPrefix(c.Text, "//go:build ") || strings.HasPrefix(c.Text, "// +build ") {
					fr.BuildConstraints = append(fr.BuildConstraints, c.Text)
				}
				if strings.TrimSpace(c.Text) == "//go:build ignore" {
					fr.Excluded = "build-ignore tool"
				}
				if strings.HasPrefix(c.Text, "//go:linkname") {
					r.Hazards = append(r.Hazards, Hazard{name, fset.Position(c.Pos()).Line, "linkname", c.Text})
				}
			}
		}
		if f.Name.Name == "main" {
			fr.Excluded = "command/tool package"
		}
		r.Files = append(r.Files, fr)
		if fr.Excluded != "" {
			continue
		}
		parsed[name] = f
		for _, im := range f.Imports {
			p, _ := strconv.Unquote(im.Path.Value)
			rec := resolveImport(s, pkg, p)
			imports[p] = rec
			if rec.Boundary == "external-unresolved" || p == "unsafe" || (im.Name != nil && (im.Name.Name == "." || im.Name.Name == "_")) {
				r.Hazards = append(r.Hazards, Hazard{name, fset.Position(im.Pos()).Line, "import-review", p})
			}
		}
		add := func(id *ast.Ident, node ast.Node, kind, initializer string) {
			key := pkg + "." + id.Name
			if fn, ok := node.(*ast.FuncDecl); ok && fn.Recv != nil {
				key = pkg + "." + nodeText(fset, fn.Recv.List[0].Type) + "." + id.Name
			}
			if id.Name == "init" || id.Name == "_" {
				key += fmt.Sprintf("@%s:%d", path.Base(name), fset.Position(id.Pos()).Line)
			}
			d := Declaration{ID: key, Kind: kind, File: name, Line: fset.Position(id.Pos()).Line, Initializer: initializer}
			if v, ok := node.(*ast.ValueSpec); ok && v.Type != nil {
				d.TypeSyntax = nodeText(fset, v.Type)
			}
			if kind == "var" {
				d.StateReview = "unclassified; explicit owner or immutable justification required"
			}
			r.Declarations = append(r.Declarations, d)
			owners[node] = append(owners[node], key)
			declIDs[id] = true
			if id.Obj != nil {
				objects[id.Obj] = key
			}
			if fn, ok := node.(*ast.FuncDecl); !ok || fn.Recv == nil {
				top[id.Name] = key
			}
		}
		for _, d := range f.Decls {
			switch d := d.(type) {
			case *ast.FuncDecl:
				add(d.Name, d, "func", "")
			case *ast.GenDecl:
				for _, sp := range d.Specs {
					switch sp := sp.(type) {
					case *ast.TypeSpec:
						add(sp.Name, sp, "type", "")
					case *ast.ValueSpec:
						for i, id := range sp.Names {
							init := ""
							if len(sp.Values) == len(sp.Names) {
								init = nodeText(fset, sp.Values[i])
							} else if len(sp.Values) > 0 {
								init = nodeText(fset, sp.Values[0])
							}
							add(id, sp, d.Tok.String(), init)
						}
					}
				}
			}
		}
	}
	for _, name := range names {
		f := parsed[name]
		if f == nil {
			continue
		}
		aliases := map[string]string{}
		for _, im := range f.Imports {
			p, _ := strconv.Unquote(im.Path.Value)
			alias := path.Base(p)
			if im.Name != nil {
				alias = im.Name.Name
			}
			aliases[alias] = imports[p].Resolved
		}
		var stack []ast.Node
		ast.Inspect(f, func(n ast.Node) bool {
			if n == nil {
				stack = stack[:len(stack)-1]
				return false
			}
			stack = append(stack, n)
			owner := []string{pkg + ".<package-init>"}
			for i := len(stack) - 1; i >= 0; i-- {
				if o := owners[stack[i]]; len(o) != 0 {
					owner = o
					break
				}
			}
			if call, ok := n.(*ast.CallExpr); ok {
				if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
					if x, ok := sel.X.(*ast.Ident); ok && x.Obj == nil {
						p := aliases[x.Name]
						if p == "os" || p == "os/exec" || p == "syscall" || p == "log" || p == "flag" || p == "unsafe" || p == "runtime" || p == "sync" {
							r.Hazards = append(r.Hazards, Hazard{name, fset.Position(n.Pos()).Line, "environment-or-shared-facility", p + "." + sel.Sel.Name})
						}
					}
				}
			}
			if _, ok := n.(*ast.GoStmt); ok {
				r.Hazards = append(r.Hazards, Hazard{name, fset.Position(n.Pos()).Line, "goroutine", "review captured state and lifetime"})
			}
			id, ok := n.(*ast.Ident)
			if !ok || declIDs[id] {
				return true
			}
			var parent ast.Node
			if len(stack) > 1 {
				parent = stack[len(stack)-2]
			}
			if sel, ok := parent.(*ast.SelectorExpr); ok && sel.Sel == id {
				return true
			}
			if _, ok := parent.(*ast.Field); ok {
				return true
			}
			if kv, ok := parent.(*ast.KeyValueExpr); ok && kv.Key == id && id.Obj == nil {
				return true
			}
			to := ""
			if id.Obj != nil {
				to = objects[id.Obj]
			} else {
				to = top[id.Name]
			}
			if sel, ok := parent.(*ast.SelectorExpr); ok && sel.X == id && id.Obj == nil {
				if p := aliases[id.Name]; p != "" {
					to = p + "." + sel.Sel.Name
				}
			}
			if to != "" {
				for _, from := range owner {
					r.References = append(r.References, Reference{from, to, name, fset.Position(id.Pos()).Line, referenceUse(stack)})
				}
			}
			return true
		})
	}
	deps := map[string]map[string]bool{}
	for _, ref := range r.References {
		if deps[ref.From] == nil {
			deps[ref.From] = map[string]bool{}
		}
		deps[ref.From][ref.To] = true
	}
	for i := range r.Declarations {
		d := &r.Declarations[i]
		if d.Kind == "var" && d.Initializer != "" {
			d.InitializerDependencies = sortedKeys(deps[d.ID])
		}
	}
	for _, im := range imports {
		r.Imports = append(r.Imports, im)
	}
	sort.Slice(r.Imports, func(i, j int) bool { return r.Imports[i].Path < r.Imports[j].Path })
	return r, nil
}

func referenceUse(stack []ast.Node) string {
	id := stack[len(stack)-1]
	for i := len(stack) - 2; i >= 0; i-- {
		n := stack[i]
		switch n := n.(type) {
		case *ast.UnaryExpr:
			if n.Op == token.AND {
				return "address-escape"
			}
		case *ast.IncDecStmt:
			return "read-write"
		case *ast.AssignStmt:
			for _, lhs := range n.Lhs {
				if lhs.Pos() <= id.Pos() && id.End() <= lhs.End() {
					if n.Tok != token.ASSIGN && n.Tok != token.DEFINE {
						return "read-write"
					}
					if lhs == id {
						return "write"
					}
					return "write-through"
				}
			}
			return "read-or-function-value"
		case *ast.CallExpr:
			if n.Fun.Pos() <= id.Pos() && id.End() <= n.Fun.End() {
				if n.Fun == id {
					return "call"
				}
				return "call-or-receiver-mutation"
			}
			return "argument-or-escape"
		case *ast.FuncDecl, *ast.ValueSpec:
			return "read-or-function-value"
		}
	}
	return "read-or-function-value"
}

package port

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"path"
	"strings"
)

// Only upstream function signatures are needed to type-check calls whose host
// implementation is intentionally replaced. Bodies are omitted, as for external
// Go declarations; these packages exist only inside the generator's type checker.
// The real adapter's full signature is checked again after transformation.
func loadCallSignatures(s *Source, units []Unit, importer *packageImporter) (map[string]string, error) {
	selected := map[string]map[string]bool{}
	for _, u := range units {
		for _, call := range u.Calls {
			if selected[call.Source] == nil {
				selected[call.Source] = map[string]bool{}
			}
			selected[call.Source][call.Function] = true
		}
	}
	fs := token.NewFileSet()
	packages := map[string][]*ast.File{}
	hashes := map[string]string{}
	for _, source := range sortedKeys(selected) {
		data, err := s.Read(source)
		if err != nil {
			return nil, err
		}
		hashes[source] = hash(data)
		f, err := selectFile(fs, source, data, Unit{Functions: sortedKeys(selected[source])})
		if err != nil {
			return nil, err
		}
		for _, d := range f.Decls {
			if fn, ok := d.(*ast.FuncDecl); ok {
				fn.Body = nil
			}
		}
		pruneImports(f)
		pkg := strings.TrimPrefix(path.Dir(source), "src/")
		packages[pkg] = append(packages[pkg], f)
	}
	for _, pkg := range sortedKeys(packages) {
		checked, _, err := checkTypes(fs, pkg, packages[pkg], importer)
		if err != nil {
			return nil, fmt.Errorf("upstream adapter signatures: %w", err)
		}
		importer.packages[pkg] = checked
	}
	return hashes, nil
}

func checkCallAdapters(units []Unit, before *packageImporter, out *types.Package, imports map[string]string) error {
	for _, u := range units {
		for _, call := range u.Calls {
			sourcePkg := strings.TrimPrefix(path.Dir(call.Source), "src/")
			original := before.packages[sourcePkg].Scope().Lookup(call.Function)
			ctx := out.Scope().Lookup(u.Context.Type)
			if ctx == nil {
				return fmt.Errorf("missing adapter context")
			}
			adapter, _, _ := types.LookupFieldOrMethod(types.NewPointer(ctx.Type()), true, out, call.Method)
			if _, ok := adapter.(*types.Func); !ok {
				return fmt.Errorf("missing adapter method %s", call.Method)
			}
			want := types.TypeString(original.Type(), func(p *types.Package) string {
				if mapped := imports[p.Path()]; mapped != "" {
					return mapped
				}
				return p.Path()
			})
			if got := types.TypeString(adapter.Type(), nil); got != want {
				return fmt.Errorf("adapter signature mismatch for %s: got %s, want %s", call.Method, got, want)
			}
		}
	}
	return nil
}

package port

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"path"
	"path/filepath"
	"sort"
)

func renderPackage(s *Source, units []Unit, imports map[string]string, root string, before, after *packageImporter) (map[string][]byte, []Provenance, error) {
	// Go initializes files in lexical filename order, independent of plan order.
	units = append([]Unit(nil), units...)
	sort.Slice(units, func(i, j int) bool { return units[i].Source < units[j].Source })
	signatureSources, err := loadCallSignatures(s, units, before)
	if err != nil {
		return nil, nil, err
	}
	fs := token.NewFileSet()
	var original []*ast.File
	sources := make([][]byte, len(units))
	for i, u := range units {
		data, err := s.Read(u.Source)
		if err != nil {
			return nil, nil, err
		}
		sources[i] = data
		f, err := selectFile(fs, u.Source, data, u)
		if err != nil {
			return nil, nil, err
		}
		if f.Name.Name == "main" {
			return nil, nil, fmt.Errorf("command package requires explicit adaptation: %s", u.Source)
		}
		original = append(original, f)
	}
	sourcePkg := units[0].sourcePackage()
	signatures, err := localCallSignatures(s, units, fs, original)
	if err != nil {
		return nil, nil, err
	}
	checkedFiles := append(append([]*ast.File(nil), original...), signatures...)
	pkg, info, err := checkTypesImports(fs, sourcePkg, checkedFiles, before, true)
	if err != nil {
		return nil, nil, fmt.Errorf("selected package %s: %w", sourcePkg, err)
	}
	before.packages[sourcePkg] = pkg
	for _, f := range checkedFiles {
		ast.Inspect(f, func(n ast.Node) bool {
			if id, ok := n.(*ast.Ident); ok {
				if obj := info.Defs[id]; obj != nil && obj.Parent() == pkg.Scope() {
					before.sources[obj] = fs.PositionFor(id.Pos(), false).Filename
				}
			}
			return true
		})
	}
	if err := removeAdapterMethods(original, units); err != nil {
		return nil, nil, err
	}
	if err := replaceZeroValues(fs, original, units, info); err != nil {
		return nil, nil, err
	}

	for _, unit := range units {
		for _, name := range unit.ZeroValues {
			obj := pkg.Scope().Lookup(name)
			before.zeros[obj] = zeroTypeName(obj)
		}
	}
	replaceImportedZeros(original, info, before.zeros)
	if err := localizeReadTables(fs, original, info, units); err != nil {
		return nil, nil, err
	}
	if err := moveReceiverState(fs, original, info, units); err != nil {
		return nil, nil, err
	}
	if err := rewritePackageContext(fs, original, info, units); err != nil {
		return nil, nil, err
	}
	if err := extendNativeContext(original, info, units); err != nil {
		return nil, nil, err
	}
	adapters := map[string][]byte{}
	outfs := token.NewFileSet()
	var output []*ast.File
	result := make(map[string][]byte)
	var provenance []Provenance
	for i, u := range units {
		out, err := transform(fs, original[i], info, u, imports)
		if err != nil {
			return nil, nil, err
		}
		outf, err := parser.ParseFile(outfs, u.output(), out, parser.ParseComments)
		if err != nil {
			return nil, nil, err
		}
		output = append(output, outf)
		prov := Provenance{Output: u.output(), Source: u.Source, SourceSHA256: hash(sources[i]), OutputSHA256: hash(out), Rule: "select-and-relocate-v1", Declarations: append(append([]string(nil), u.Functions...), u.Declarations...)}
		for _, state := range u.State {
			if prov.SignatureSources == nil {
				prov.SignatureSources = map[string]string{}
			}
			prov.SignatureSources[state.Source] = signatureSources[state.Source]
		}
		for _, call := range u.Calls {
			if prov.SignatureSources == nil {
				prov.SignatureSources = map[string]string{}
			}
			prov.SignatureSources[call.Source] = signatureSources[call.Source]
		}
		if u.WholeFile {
			prov.Rule = "whole-file-and-zero-values-v1"
			prov.Declarations = append(append([]string{"*"}, u.ZeroValues...), u.ReadTables...)
		}
		for _, name := range u.AdapterMethods {
			prov.Declarations = append(prov.Declarations, "adapter-method:"+name)
		}
		if state := u.ReceiverState; state != nil {
			prov.Rule = "package-state-to-existing-receiver-v1"
			prov.Declarations = append(prov.Declarations, sortedKeys(state.Fields)...)
		}
		if c := u.Context; c != nil {
			prov.Rule = "package-state-to-context-methods-v2"
			prov.Declarations = append(prov.Declarations, sortedKeys(c.Fields)...)
			prov.Declarations = append(prov.Declarations, u.ContextParams...)
		}
		if c := u.Context; c != nil && c.Adapter != "" {
			if _, exists := s.Blobs[path.Join(path.Dir(u.Source), c.Adapter)]; exists {
				return nil, nil, fmt.Errorf("adapter filename collides with upstream: %s", u.adapterPath())
			}
			if err := checkPath(root, u.adapterPath()); err != nil {
				return nil, nil, err
			}
			adapter, loaded := adapters[u.adapterPath()]
			if !loaded {
				adapter, err = os.ReadFile(filepath.Join(root, u.adapterPath()))
				if err != nil {
					return nil, nil, err
				}
				af, err := parser.ParseFile(outfs, u.adapterPath(), adapter, parser.ParseComments)
				if err != nil {
					return nil, nil, err
				}
				output = append(output, af)
				adapters[u.adapterPath()] = adapter
			}
			prov.AdapterSHA256 = hash(adapter)
		}
		result[u.output()] = out
		provenance = append(provenance, prov)
	}
	dest := imports[sourcePkg]
	outpkg, outinfo, err := checkTypes(outfs, dest, output, after)
	if err != nil {
		return nil, nil, fmt.Errorf("transformed package %s: %w", dest, err)
	}
	if err := checkAdapterMethods(units, pkg, outpkg, imports, outfs); err != nil {
		return nil, nil, err
	}
	if err := auditOutput(outpkg, output, outinfo); err != nil {
		return nil, nil, err
	}
	for _, u := range units {
		if c := u.Context; c != nil {
			obj := outpkg.Scope().Lookup(c.Type)
			if obj == nil {
				return nil, nil, fmt.Errorf("missing context %s", c.Type)
			}
			st, ok := obj.Type().Underlying().(*types.Struct)
			if !ok {
				return nil, nil, fmt.Errorf("context is not a struct")
			}
			for name, field := range c.Fields {
				var actual types.Type
				for i := 0; i < st.NumFields(); i++ {
					if st.Field(i).Name() == field {
						actual = st.Field(i).Type()
					}
				}
				if actual == nil || types.TypeString(actual, nil) != types.TypeString(pkg.Scope().Lookup(name).Type(), func(p *types.Package) string {
					if dest := imports[p.Path()]; dest != "" {
						return dest
					}
					return p.Path()
				}) {
					return nil, nil, fmt.Errorf("context field %s does not preserve state type of %s", field, name)
				}
			}
		}
	}
	if err := checkEntryAdapters(units, pkg, outpkg, imports); err != nil {
		return nil, nil, err
	}
	if err := checkStateAdapters(units, before, outpkg, imports); err != nil {
		return nil, nil, err
	}
	if err := checkTraceAdapters(units, info, outpkg); err != nil {
		return nil, nil, err
	}
	if err := checkCallAdapters(units, before, outpkg, imports); err != nil {
		return nil, nil, err
	}
	after.packages[dest] = outpkg
	return result, provenance, nil
}

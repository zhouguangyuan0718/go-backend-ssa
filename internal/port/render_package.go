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
	"strings"
)

func renderPackage(s *Source, units []Unit, imports map[string]string, root string, before, after *packageImporter) (map[string][]byte, []Provenance, error) {
	// Go initializes files in lexical filename order, independent of plan order.
	units = append([]Unit(nil), units...)
	sort.Slice(units, func(i, j int) bool { return units[i].Source < units[j].Source })
	signatureSources, err := loadCallSignatures(s, units, before)
	if err != nil {
		return nil, nil, err
	}
	if err := loadMethodCallSignatures(s, units, before, signatureSources); err != nil {
		return nil, nil, err
	}
	fs := token.NewFileSet()
	var original []*ast.File
	regionSources := make([]map[string]string, len(units))
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
		regionSources[i], err = extractFunctionRegions(s, fs, f, u, before)
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
	inputSignatures, err := localInputSignatures(s, units, fs, before)
	if err != nil {
		return nil, nil, err
	}
	signatures = append(signatures, inputSignatures...)
	checkedFiles := append(append([]*ast.File(nil), original...), signatures...)
	pkg, info, err := checkTypesImports(fs, sourcePkg, checkedFiles, before, true)
	if err != nil {
		return nil, nil, fmt.Errorf("selected package %s: %w", sourcePkg, err)
	}
	if err := checkFunctionRegionInputs(original, info, units); err != nil {
		return nil, nil, err
	}
	if err := checkLocalInputs(original, info, pkg, units); err != nil {
		return nil, nil, err
	}
	before.packages[sourcePkg] = pkg
	for _, file := range original {
		for _, decl := range file.Decls {
			if fn, ok := decl.(*ast.FuncDecl); ok && fn.Recv != nil {
				obj := info.Defs[fn.Name]
				if functionRecovers(fn, info) {
					before.recovering[obj] = true
				}
			}
		}
	}
	for _, f := range checkedFiles {
		ast.Inspect(f, func(n ast.Node) bool {
			if id, ok := n.(*ast.Ident); ok {
				if obj := info.Defs[id]; obj != nil && (obj.Parent() == pkg.Scope() || nativeMethodObject(obj)) {
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
	constantHelpers, err := projectConstantFields(fs, original, info, units)
	if err != nil {
		return nil, nil, err
	}
	if err := localizeReadTables(fs, original, info, units); err != nil {
		return nil, nil, err
	}
	if err := moveReceiverState(fs, original, info, units); err != nil {
		return nil, nil, err
	}
	if err := rewriteHostSignatures(original, info, units); err != nil {
		return nil, nil, err
	}
	if err := rewritePackageContext(fs, original, info, units, before.recovering); err != nil {
		return nil, nil, err
	}
	if err := rewriteHostFields(original, info, units); err != nil {
		return nil, nil, err
	}
	if err := extendNativeContext(original, info, units); err != nil {
		return nil, nil, err
	}
	// Helpers contain only constants and primitive operations. Append them after
	// context binding, whose object identities belong to the original source.
	for i, helpers := range constantHelpers {
		original[i].Decls = append(original[i].Decls, helpers...)
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

		if len(u.FunctionRegions) != 0 {
			prov.SignatureSources = regionSources[i]
			for _, name := range sortedKeys(u.FunctionRegions) {
				r := u.FunctionRegions[name]
				prov.Declarations = append(prov.Declarations, "function-region:"+name+":"+r.First.Package+"."+r.First.Member+":"+r.Last.Package+"."+r.Last.Member+":"+strings.Join(r.Inputs, ","))
			}
		}
		for _, state := range u.State {
			if prov.SignatureSources == nil {
				prov.SignatureSources = map[string]string{}
			}
			prov.SignatureSources[state.Source] = signatureSources[state.Source]
			if state.TypeSource != "" {
				data, err := s.Read(state.TypeSource)
				if err != nil {
					return nil, nil, err
				}
				prov.SignatureSources[state.TypeSource] = hash(data)
			}
		}
		for _, call := range u.Calls {
			if prov.SignatureSources == nil {
				prov.SignatureSources = map[string]string{}
			}
			prov.SignatureSources[call.Source] = signatureSources[call.Source]
			if call.KeepDefinition {
				prov.Declarations = append(prov.Declarations, "retained-native-call:"+call.Function+":"+call.Method)
			}
		}
		if u.BuildVariant != "" {
			prov.Declarations = append(prov.Declarations, "selected-build-variant:"+u.BuildVariant)
		}
		for _, r := range u.MethodCalls {
			if prov.SignatureSources == nil {
				prov.SignatureSources = map[string]string{}
			}
			prov.SignatureSources[r.Source] = signatureSources[r.Source]
		}
		for _, call := range u.ContextParamCalls {
			data, err := s.Read(call.Source)
			if err != nil {
				return nil, nil, err
			}
			if prov.SignatureSources == nil {
				prov.SignatureSources = map[string]string{}
			}
			prov.SignatureSources[call.Source] = hash(data)
			prov.Declarations = append(prov.Declarations, "external-context-parameter:"+call.Method+":"+call.Field)
		}
		for _, literal := range u.Literals {
			if literal.MakeSlice {
				prov.Declarations = append(prov.Declarations, "make-slice:"+literal.Type+":"+literal.Method)
			}
			if literal.NilPointer {
				prov.Declarations = append(prov.Declarations, "nil-pointer:"+literal.Type+":"+literal.Method)
			}
			data, err := s.Read(literal.Source)
			if err != nil {
				return nil, nil, err
			}
			if prov.SignatureSources == nil {
				prov.SignatureSources = map[string]string{}
			}
			prov.SignatureSources[literal.Source] = hash(data)
		}
		if u.WholeFile {
			prov.Rule = "whole-file-and-zero-values-v1"
			prov.Declarations = append(append([]string{"*"}, u.ZeroValues...), u.ReadTables...)
		}
		for _, r := range u.MethodCalls {
			prov.Declarations = append(prov.Declarations, "method-call:"+r.Method+":"+r.Adapter)
		}
		for _, rule := range u.FixedSizeof {
			prov.Declarations = append(prov.Declarations, "fixed-sizeof:"+rule.Function+":"+rule.Type)
		}
		for _, state := range u.State {
			if state.Input {
				prov.Declarations = append(prov.Declarations, "host-input:"+state.Variable+":"+state.Field)
			}
		}
		for _, rule := range u.HostSignatures {
			prov.Declarations = append(prov.Declarations, fmt.Sprintf("host-stream-signature:%s:%t:%d:%s", rule.Function, rule.Result, rule.Index, strings.Join(rule.Methods, ",")))
		}
		for _, rule := range u.HostFields {
			prov.Declarations = append(prov.Declarations, "host-stream-field:"+rule.Type+"."+rule.Field+":"+strings.Join(rule.Methods, ","))
		}
		for _, r := range u.HostConstants {
			prov.Declarations = append(prov.Declarations, "host-constant:"+r.Package+"."+r.Constant+":"+r.Field)
		}
		for _, rule := range u.HostCalls {
			prov.Declarations = append(prov.Declarations, "host-call:"+rule.Package+"."+rule.Function+":"+rule.Method+":"+strings.Join(rule.stringArguments(), ","))
			if len(rule.StreamMethods) != 0 {
				prov.Declarations = append(prov.Declarations, "host-factory-stream:"+rule.Package+"."+rule.Function+":"+strings.Join(rule.StreamMethods, ","))
			}
		}
		for _, name := range u.AdapterMethods {
			prov.Declarations = append(prov.Declarations, "adapter-method:"+name)
		}
		if state := u.ReceiverState; state != nil {
			prov.Rule = "package-state-to-existing-receiver-v1"
			prov.Declarations = append(prov.Declarations, sortedKeys(state.Fields)...)
		}
		if c := u.Context; c != nil {
			if c.InitGroup != "" {
				prov.Declarations = append(prov.Declarations, "initializer-group:"+c.InitGroup)
			}
			prov.Rule = "package-state-to-context-methods-v2"
			prov.Declarations = append(prov.Declarations, sortedKeys(c.Fields)...)
			prov.Declarations = append(prov.Declarations, u.ContextParams...)
			for _, name := range u.ContextParamExports {
				prov.Declarations = append(prov.Declarations, "exported-context-parameter:"+name)
			}
			if c.DeferInit {
				prov.Declarations = append(prov.Declarations, "deferred-unselected-init")
			}
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
	sliceOperations, err := reviewedSliceOperations(outfs, output, outinfo, units)
	if err != nil {
		return nil, nil, err
	}
	resourceOperations, err := reviewedHostResources(output, info, outinfo, outpkg, units)
	if err != nil {
		return nil, nil, err
	}
	sizeOperations, err := reviewedFixedSizeof(outfs, output, outinfo, outpkg, units)
	if err != nil {
		return nil, nil, err
	}
	hostOperations, err := reviewedHostCalls(output, info, outinfo, outpkg, units)
	if err != nil {
		return nil, nil, err
	}
	if err := auditOutput(outpkg, output, outinfo, sliceOperations, resourceOperations, sizeOperations, hostOperations); err != nil {
		return nil, nil, err
	}
	for _, u := range units {
		if rule := u.ReceiverState; rule != nil {
			owner := outpkg.Scope().Lookup(rule.Type)
			if owner == nil {
				return nil, nil, fmt.Errorf("missing receiver state target %s", rule.Type)
			}
			for name, field := range rule.Fields {
				member, _, _ := types.LookupFieldOrMethod(types.NewPointer(owner.Type()), true, outpkg, field)
				actual, ok := member.(*types.Var)
				expected := pkg.Scope().Lookup(name)
				if !ok || !actual.IsField() || expected == nil || types.TypeString(actual.Type(), nil) != types.TypeString(expected.Type(), func(p *types.Package) string {
					if dst := imports[p.Path()]; dst != "" {
						return dst
					}
					return p.Path()
				}) {
					return nil, nil, fmt.Errorf("receiver field %s does not preserve state type of %s", field, name)
				}
			}
		}
		if c := u.Context; c != nil {
			obj := outpkg.Scope().Lookup(c.Type)
			if obj == nil {
				return nil, nil, fmt.Errorf("missing context %s", c.Type)
			}
			_, ok := obj.Type().Underlying().(*types.Struct)
			if !ok {
				return nil, nil, fmt.Errorf("context is not a struct")
			}
			for name, field := range c.Fields {
				var actual types.Type
				member, _, _ := types.LookupFieldOrMethod(types.NewPointer(obj.Type()), true, outpkg, field)
				if v, ok := member.(*types.Var); ok && v.IsField() {
					actual = v.Type()
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
	if err := checkHostConstants(units, info, outpkg); err != nil {
		return nil, nil, err
	}
	if err := checkStateAdapters(units, before, outpkg, imports); err != nil {
		return nil, nil, err
	}
	if err := checkTraceAdapters(units, info, outpkg); err != nil {
		return nil, nil, err
	}
	if err := checkMethodCallAdapters(units, before, outpkg, imports); err != nil {
		return nil, nil, err
	}
	if err := checkCallAdapters(units, before, outpkg, imports); err != nil {
		return nil, nil, err
	}
	if err := checkLiteralAdapters(units, before, outpkg, imports); err != nil {
		return nil, nil, err
	}
	after.packages[dest] = outpkg
	return result, provenance, nil
}

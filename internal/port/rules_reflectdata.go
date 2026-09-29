package port

func reflectdataMigration() []Unit {
	context := func() *ContextRule {
		return &ContextRule{Type: "Context", Constructor: "newContext", Adapter: "adapter_context.go", Selective: true}
	}
	reflect := Unit{
		Source:       "src/cmd/compile/internal/reflectdata/reflect.go",
		Context:      context(),
		Declarations: []string{"typeAndStr"},
		Functions:    []string{"TrackSym", "TypeSymPrefix", "TypeSym", "TypeLinksymPrefix", "TypeLinksymLookup", "TypeLinksym", "TypePtrAt", "NeedRuntimeType", "writtenByWriteBasicTypes", "NeedEmit", "typesStrCmp", "MarkTypeUsedInInterface", "MarkTypeSymUsedInInterface"},
		Checks:       map[string]string{"TypeSym": "checkType", "TypeSymPrefix": "checkPrefix", "TypeLinksym": "checkType", "TypeLinksymPrefix": "checkPrefix", "TypePtrAt": "checkTypeAt", "NeedRuntimeType": "checkType", "NeedEmit": "checkType", "MarkTypeUsedInInterface": "checkFrom", "TrackSym": "checkField"},
		State:        []StateRule{{Source: "src/cmd/compile/internal/base/link.go", Variable: "Ctxt", Field: "Typecheck.IR.Base.Ctxt"}},
		Calls: []CallRule{
			{Source: "src/cmd/compile/internal/base/print.go", Function: "Fatalf", Method: "fatalf"},
			{Source: "src/cmd/compile/internal/base/print.go", Function: "AssertfAt", Method: "assertfAt"},
			{Source: "src/cmd/compile/internal/base/link.go", Function: "PkgLinksym", Method: "Typecheck.IR.Base.PkgLinksym"},
			{Source: "src/cmd/compile/internal/typecheck/subr.go", Function: "LinksymAddr", Method: "Typecheck.LinksymAddr"},
		},
	}
	reflect.Context.Fields = map[string]string{"signatmu": "signatmu", "signatset": "signatset", "signatslice": "signatslice"}
	for _, name := range []string{"Types", "ErrorType"} {
		reflect.State = append(reflect.State, StateRule{Source: "src/cmd/compile/internal/types/type.go", Variable: name, Field: "Typecheck.IR.Types." + name})
	}
	for _, name := range []string{"BuiltinPkg", "LocalPkg", "UnsafePkg"} {
		reflect.State = append(reflect.State, StateRule{Source: "src/cmd/compile/internal/types/fmt.go", Variable: name, Field: "Typecheck.IR.Types." + name})
	}
	for _, name := range []string{"TypeSym", "TypeSymLookup", "TypeSymName"} {
		reflect.Calls = append(reflect.Calls, CallRule{Source: "src/cmd/compile/internal/types/type.go", Function: name, Method: "Typecheck.IR.Types." + name})
	}
	// ConvIfaceTypeWord also emits non-empty-interface ITabs. Preserve that full
	// native path in the later descriptor batch rather than substituting a stub.
	helpers := Unit{Source: "src/cmd/compile/internal/reflectdata/helpers.go", Context: context(), Functions: []string{
		"hasRType", "assertOp", "assertOp2", "kindRType", "mapRType", "chanRType", "sliceElemRType", "concreteRType",
		"AppendElemRType", "CompareRType", "ConvIfaceSrcRType", "CopyElemRType", "DeleteMapRType", "IndexMapRType", "MakeChanRType", "MakeMapRType", "MakeSliceElemRType", "RangeMapRType", "UnsafeSliceElemRType",
	}}
	mapTypes := Unit{Source: "src/cmd/compile/internal/reflectdata/map.go", Context: context(), Functions: []string{"MapGroupType", "mapTableType", "MapType", "MapIterType"}, Checks: map[string]string{"MapGroupType": "checkType"}}
	mapTypes.Context.Fields = map[string]string{"cachedMapTableType": "cachedMapTableType", "cachedMapType": "cachedMapType", "cachedMapIterType": "cachedMapIterType"}
	mapTypes.State = []StateRule{
		{Source: "src/cmd/compile/internal/types/size.go", Variable: "PtrSize", Field: "ptrSize"},
		{Source: "src/cmd/compile/internal/ir/symtab.go", Variable: "Pkgs", Field: "Typecheck.IR.Pkgs"},
		{Source: "src/internal/buildcfg/exp.go", Variable: "Experiment", Field: "Typecheck.IR.Types.Experiment"},
	}
	mapTypes.Calls = []CallRule{
		{Source: "src/cmd/compile/internal/reflectdata/reflect.go", Function: "makefield", Method: "makefield"},
		{Source: "src/cmd/compile/internal/types/size.go", Function: "CalcSize", Method: "Typecheck.IR.Types.CalcSize"},
		{Source: "src/cmd/compile/internal/types/alg.go", Function: "IsComparable", Method: "Typecheck.IR.Types.IsComparable"},
		{Source: "src/cmd/compile/internal/ir/name.go", Function: "NewDeclNameAt", Method: "Typecheck.IR.NewDeclNameAt"},
	}
	for _, name := range []string{"NewPtr", "NewArray", "NewStruct", "NewNamed"} {
		mapTypes.Calls = append(mapTypes.Calls, CallRule{Source: "src/cmd/compile/internal/types/type.go", Function: name, Method: "Typecheck.IR.Types." + name})
	}
	return []Unit{reflect, helpers, mapTypes}
}

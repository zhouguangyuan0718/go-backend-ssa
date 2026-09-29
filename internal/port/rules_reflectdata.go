package port

func reflectdataMigration() []Unit {
	context := func() *ContextRule {
		return &ContextRule{Type: "Context", Constructor: "newContext", Adapter: "adapter_context.go", Selective: true, RedirectTrace: true}
	}
	reflect := Unit{
		Source:       "src/cmd/compile/internal/reflectdata/reflect.go",
		Context:      context(),
		Declarations: []string{"typeAndStr", "typeSig", "ptabEntry"},
		Functions:    []string{"commonSize", "uncommonSize", "methods", "imethods", "dimportpath", "dgopkgpath", "dgopkgpathOff", "dnameField", "dnameData", "dname", "dextratype", "typePkg", "dmethodptrOff", "ABIKindOfType", "dcommontype", "TrackSym", "TypeSymPrefix", "TypeSym", "TypeLinksymPrefix", "TypeLinksymLookup", "TypeLinksym", "TypePtrAt", "ITabLsym", "itabLsym", "ITabAddrAt", "needkeyupdate", "hashMightPanic", "formalType", "writeType", "InterfaceMethodOffset", "NeedRuntimeType", "WriteRuntimeTypes", "WriteGCSymbols", "writeITab", "WritePluginTable", "writtenByWriteBasicTypes", "WriteBasicTypes", "typesStrCmp", "GCSym", "dgcsym", "dgcptrmask", "fillptrmask", "dgcptrmaskOnDemand", "ZeroAddr", "NeedEmit", "methodWrapper", "MarkTypeUsedInInterface", "MarkTypeSymUsedInInterface", "MarkUsedIfaceMethod"},
		Checks:       map[string]string{"GCSym": "checkGCType", "TypeSym": "checkType", "TypeSymPrefix": "checkPrefix", "TypeLinksym": "checkType", "TypeLinksymPrefix": "checkPrefix", "TypePtrAt": "checkTypeAt", "NeedRuntimeType": "checkType", "NeedEmit": "checkType", "MarkTypeUsedInInterface": "checkFrom", "TrackSym": "checkField"},
		State:        []StateRule{{Source: "src/cmd/compile/internal/base/link.go", Variable: "Ctxt", Field: "Typecheck.IR.Base.Ctxt"}},
		Calls: []CallRule{
			{Source: "src/cmd/compile/internal/base/print.go", Function: "Fatalf", Method: "fatalf"},
			{Source: "src/cmd/compile/internal/base/print.go", Function: "AssertfAt", Method: "assertfAt"},
			{Source: "src/cmd/compile/internal/base/link.go", Function: "PkgLinksym", Method: "Typecheck.IR.Base.PkgLinksym"},
			{Source: "src/cmd/compile/internal/typecheck/subr.go", Function: "LinksymAddr", Method: "Typecheck.LinksymAddr"},
		},
	}
	reflect.Context.Fields = map[string]string{"kinds": "kinds", "dnameCount": "dnameCount", "signatmu": "signatmu", "signatset": "signatset", "signatslice": "signatslice", "gcsymmu": "gcsymmu", "gcsymset": "gcsymset", "ZeroSize": "ZeroSize", "memhashvarlen": "memhashvarlen", "memequalvarlen": "memequalvarlen"}
	for _, name := range []string{"Types", "ErrorType", "AnyType", "ByteType", "RuneType"} {
		reflect.State = append(reflect.State, StateRule{Source: "src/cmd/compile/internal/types/type.go", Variable: name, Field: "Typecheck.IR.Types." + name})
	}
	for _, name := range []string{"BuiltinPkg", "LocalPkg", "UnsafePkg"} {
		reflect.State = append(reflect.State, StateRule{Source: "src/cmd/compile/internal/types/fmt.go", Variable: name, Field: "Typecheck.IR.Types." + name})
	}
	for _, name := range []string{"TypeSym", "TypeSymLookup", "TypeSymName"} {
		reflect.Calls = append(reflect.Calls, CallRule{Source: "src/cmd/compile/internal/types/type.go", Function: name, Method: "Typecheck.IR.Types." + name})
	}

	reflect.State = append(reflect.State, StateRule{Source: "src/cmd/compile/internal/base/print.go", Variable: "Pos", Field: "Typecheck.IR.Types.Pos"})
	reflect.Calls = append(reflect.Calls,
		CallRule{Source: "src/cmd/compile/internal/types/size.go", Function: "PtrDataSize", Method: "Typecheck.IR.Types.PtrDataSize"},
		CallRule{Source: "src/cmd/compile/internal/ir/expr.go", Function: "NewLinksymExpr", Method: "Typecheck.IR.NewLinksymExpr"},
		CallRule{Source: "src/cmd/compile/internal/typecheck/typecheck.go", Function: "Expr", Method: "Typecheck.Expr"},
		CallRule{Source: "src/cmd/compile/internal/typecheck/subr.go", Function: "NodAddr", Method: "Typecheck.NodAddr"},
	)
	for _, name := range []string{"Uint8", "Global"} {
		reflect.Calls = append(reflect.Calls, CallRule{Source: "src/cmd/compile/internal/objw/objw.go", Function: name, Method: "Runtime.Writer." + name})
	}
	helpers := Unit{Source: "src/cmd/compile/internal/reflectdata/helpers.go", Context: context(), WholeFile: true}
	mapTypes := Unit{Source: "src/cmd/compile/internal/reflectdata/map.go", Context: context(), WholeFile: true, Checks: map[string]string{"MapGroupType": "checkType"}}
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
	reflectDescriptors(&reflect)
	return []Unit{reflect, helpers, mapTypes, reflectAlgorithms(context())}
}

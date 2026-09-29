package port

func reflectDescriptors(u *Unit) {
	u.Checks["ITabLsym"] = "checkITabTypes"
	u.Checks["ITabAddrAt"] = "checkITabAt"
	u.Checks["InterfaceMethodOffset"] = "checkMethodOffset"
	u.Checks["writeType"] = "checkType"
	u.State = append(u.State, StateRule{Source: "src/cmd/compile/internal/typecheck/target.go", Variable: "Target", Field: "Typecheck.Target"})
	for _, name := range []string{"Type", "ArrayType", "ChanType", "FuncType", "InterfaceType", "MapType", "PtrType", "SliceType", "StructType", "IMethod", "Method", "StructField", "UncommonType", "ITab"} {
		u.State = append(u.State, StateRule{Source: "src/cmd/compile/internal/rttype/rttype.go", Variable: name, Field: "Runtime." + name})
	}
	for _, group := range []struct {
		file, prefix string
		names        []string
	}{
		{"types/type.go", "Typecheck.IR.Types.", []string{"IsDirectIface", "ReceiverBaseType", "IsMethodApplicable", "NewSlice"}},
		{"types/alg.go", "Typecheck.IR.Types.", []string{"TypeHasNoAlg"}},
		{"types/fmt.go", "Typecheck.IR.Types.", []string{"TypeHash"}},
		{"types/size.go", "Typecheck.IR.Types.", []string{"RoundUp"}},
		{"typecheck/subr.go", "Typecheck.", []string{"CalcMethods"}},
		{"typecheck/dcl.go", "Typecheck.", []string{"NewMethodType"}},
		{"typecheck/iimport.go", "Typecheck.", []string{"BaseTypeIndex"}},
		{"ir/expr.go", "Typecheck.IR.", []string{"MethodSym"}},
		{"rttype/rttype.go", "Runtime.", []string{"NewCursor", "NewArrayCursor"}},
		{"objw/objw.go", "Runtime.Writer.", []string{"SymPtrOff"}},
		{"staticdata/data.go", "StaticData.", []string{"StringSymNoCommon"}},
	} {
		for _, name := range group.names {
			u.Calls = append(u.Calls, CallRule{Source: "src/cmd/compile/internal/" + group.file, Function: name, Method: group.prefix + name})
		}
	}
}

func compareMigration() []Unit {
	return []Unit{{Source: "src/cmd/compile/internal/compare/compare.go", Functions: []string{"IsRegularMemory"},
		Context:     &ContextRule{Type: "typeContext", Constructor: "newTypeContext", Adapter: "adapter_context.go", Selective: true},
		ContextFrom: map[string]ContextSource{"IsRegularMemory": {Argument: "t", Resolver: "typeContextOf"}},
		Calls:       []CallRule{{Source: "src/cmd/compile/internal/types/alg.go", Function: "AlgType", Method: "Types.AlgType"}},
	}}
}

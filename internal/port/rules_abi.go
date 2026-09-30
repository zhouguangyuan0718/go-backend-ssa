package port

func abiMigration() []Unit {
	const root = "src/cmd/compile/internal/"
	return []Unit{{Source: root + "abi/abiutils.go", WholeFile: true,
		Context:       &ContextRule{Type: "Context", Constructor: "newContext", Selective: true, PreserveNilArgs: true, Adapter: "adapter_context.go", Fields: map[string]string{"synthOnce": "synthOnce", "synthSlice": "synthSlice", "synthString": "synthString", "synthIface": "synthIface"}},
		Owner:         &OwnerRule{Types: []string{"ABIConfig", "ABIParamAssignment"}, Context: "Context", Field: "compilation"},
		ContextFrom:   map[string]ContextSource{"ABIParamAssignment.Offset": {Argument: "a", Resolver: "assignmentContext"}, "ABIParamAssignment.RegisterTypesAndOffsets": {Argument: "pa", Resolver: "assignmentContext"}, "ABIParamAssignment.FrameOffset": {Argument: "a", Resolver: "assignmentContext"}, "ABIParamAssignment.ToString": {Argument: "ri", Resolver: "assignmentContext"}, "ABIParamAssignment.ComputePadding": {Argument: "pa", Resolver: "assignmentContext"}},
		ContextParams: []string{"assignState.*"},
		Checks:        map[string]string{"ABIConfig.NumParamRegs": "checkType", "ABIConfig.ABIAnalyzeTypes": "checkTypes", "ABIConfig.ABIAnalyzeFuncType": "checkSignature", "ABIConfig.ABIAnalyze": "checkAnalyze", "RegisterTypes": "checkAssignments", "ABIParamAssignment.FrameOffset": "checkInfo"},
		State: []StateRule{
			{Source: root + "types/type.go", Variable: "Types", Field: "IR.Types.Types"},
			{Source: root + "types/size.go", Variable: "RegSize", Field: "regSize"},
			{Source: root + "types/fmt.go", Variable: "BuiltinPkg", Field: "IR.Types.BuiltinPkg"},
		},
		Calls: []CallRule{
			{Source: root + "types/size.go", Function: "RoundUp", Method: "IR.Types.RoundUp"},
			{Source: root + "base/print.go", Function: "Fatalf", Method: "fatalf"},
			{Source: root + "base/print.go", Function: "FmtPos", Method: "IR.Base.FmtPos"},
			{Source: root + "types/type.go", Function: "FloatForComplex", Method: "IR.Types.FloatForComplex"},
			{Source: root + "types/type.go", Function: "NewPtr", Method: "IR.Types.NewPtr"},
			{Source: root + "types/type.go", Function: "NewStruct", Method: "IR.Types.NewStruct"},
			{Source: root + "types/type.go", Function: "NewField", Method: "IR.Types.NewField"},
			{Source: root + "types/size.go", Function: "CalcStructSize", Method: "IR.Types.CalcStructSize"},
		},
	}}
}

package port

func ssaAuxUnit() Unit {
	const root = "src/cmd/compile/internal/"
	return Unit{Source: root + "ssa/op.go",
		Declarations: []string{"Op", "opInfo", "inputInfo", "outputInfo", "regInfo", "regInfo.String", "auxType", "auxNone", "SymEffect", "SymRead",
			"AuxNameOffset", "AuxNameOffset.CanBeAnSSAAux", "AuxNameOffset.String", "AuxNameOffset.FrameOffset",
			"AuxCall", "AuxCall.Reg", "AuxCall.ABI", "AuxCall.ABIInfo", "AuxCall.ResultReg", "AuxCall.ArgWidth", "AuxCall.ParamAssignmentForResult", "AuxCall.OffsetOfResult", "AuxCall.OffsetOfArg", "AuxCall.RegsOfResult", "AuxCall.RegsOfArg", "AuxCall.NameOfResult", "AuxCall.TypeOfResult", "AuxCall.TypeOfArg", "AuxCall.SizeOfResult", "AuxCall.SizeOfArg", "AuxCall.NResults", "AuxCall.NArgs", "AuxCall.LateExpansionResultType", "AuxCall.String", "AuxCall.CanBeAnSSAAux",
			"Sym", "ValAndOff", "ValAndOff.Val", "ValAndOff.Val64", "ValAndOff.Val16", "ValAndOff.Val8", "ValAndOff.Off", "ValAndOff.Off64", "ValAndOff.String", "ValAndOff.canAdd32", "ValAndOff.canAdd64", "ValAndOff.addOffset32", "ValAndOff.addOffset64", "int128", "arm64BitField", "arm64ConditionalParams"},
		Functions:   []string{"archRegForAbiReg", "ObjRegForAbiReg", "StaticAuxCall", "InterfaceAuxCall", "ClosureAuxCall", "OwnAuxCall", "validVal", "makeValAndOff"},
		Context:     &ContextRule{Type: "Context", Constructor: "initialize", Initialize: true, Adapter: "adapter_context.go", Selective: true},
		ContextFrom: map[string]ContextSource{"AuxCall.LateExpansionResultType": {Argument: "a", Resolver: "auxCallContext"}},
		State:       []StateRule{{Source: root + "types/type.go", Variable: "TypeMem", Field: "Types.TypeMem"}},
		Calls:       []CallRule{{Source: root + "types/type.go", Function: "NewResults", Method: "Types.NewResults"}},
	}
}

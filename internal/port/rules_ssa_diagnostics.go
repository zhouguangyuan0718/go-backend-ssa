package port

func ssaDiagnosticMigration(units []Unit) []Unit {
	const root = "src/cmd/compile/internal/ssa/"
	auxQueries := []string{"Value.AuxInt8", "Value.AuxUInt8", "Value.AuxInt16", "Value.AuxInt32", "Value.AuxFloat", "Value.AuxValAndOff", "Value.AuxArm64BitField", "Value.AuxArm64ConditionalParams", "Value.auxString"}
	for i := range units {
		u := &units[i]
		switch u.Source {
		case root + "func.go":
			u.Declarations = append(u.Declarations, "Func.Fatalf")
		case root + "opGen.go":
			u.ConstantFields = append(u.ConstantFields, ConstantFieldRule{Variable: "opcodeTable", Field: "auxType", Functions: auxQueries}, ConstantFieldRule{Variable: "opcodeTable", Field: "nilCheck", Functions: []string{"liveValues", "Value.removeable"}}, ConstantFieldRule{Variable: "opcodeTable", Field: "addrSinkArg0", Functions: []string{"Value.AddrSinkArg"}}, ConstantFieldRule{Variable: "opcodeTable", Field: "addrSinkArg1", Functions: []string{"Value.AddrSinkArg"}})
			for j := range u.ConstantFields {
				if u.ConstantFields[j].Field == "call" || u.ConstantFields[j].Field == "hasSideEffects" {
					u.ConstantFields[j].Functions = append(u.ConstantFields[j].Functions, "liveValues")
				}
			}
		case root + "html.go":
			u.Declarations = append(u.Declarations, "HTMLWriter.Fatalf", "HTMLWriter.Logf", "HTMLWriter.start", "HTMLWriter.Close", "HTMLWriter.WritePhase", "HTMLWriter.flushPhases", "HTMLWriter.WriteSources", "HTMLWriter.WriteAST", "HTMLWriter.WriteColumn", "HTMLWriter.WriteMultiTitleColumn", "HTMLWriter.Printf", "HTMLWriter.WriteString", "FuncLines", "Value.HTML", "Value.LongHTML", "Block.HTML", "Block.LongHTML", "Func.HTML", "dotWriter.writeFuncSVG", "dotWriter.copyUntil", "Block.unlikelyIndex", "htmlFuncPrinter", "htmlFuncPrinter.header", "htmlFuncPrinter.startBlock", "htmlFuncPrinter.endBlock", "htmlFuncPrinter.value", "htmlFuncPrinter.startDepCycle", "htmlFuncPrinter.endDepCycle", "htmlFuncPrinter.named")
			u.Functions = []string{"ByTopoCmp"}
			u.Context = &ContextRule{Type: "Context", Constructor: "initialize", Initialize: true, Selective: true, Adapter: "adapter_context.go"}
			u.ContextFrom = map[string]ContextSource{"dotWriter.writeFuncSVG": {Argument: "f", Resolver: "diagnosticContext"}, "HTMLWriter.Close": {Argument: "w", Resolver: "htmlWriterContext"}}
			u.HostResources = []HostResourceRule{{Package: "os/exec", Factory: "Command", Method: "command", Members: []string{"StdinPipe", "Start", "Wait", "Stdout", "Stderr"}}}
		}
		if u.sourcePackage() == "cmd/compile/internal/ssa" && u.Context != nil {
			u.Context.RedirectTrace = true
		}
	}
	return append(units, Unit{Source: root + "print.go", WholeFile: true}, Unit{Source: root + "deadcode.go", Functions: []string{"findlive", "ReachableBlocks", "liveValues"}})
}

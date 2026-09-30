package port

// Preserve the native lowered-graph pipeline through scheduling and allocation.
func ssaAllocationMigration(units []Unit) []Unit {
	const root = "src/cmd/compile/internal/"
	context := func() *ContextRule {
		return &ContextRule{Type: "Context", Constructor: "initialize", Initialize: true, Selective: true, RedirectTrace: true, Adapter: "adapter_context.go"}
	}
	var states []StateRule
	var calls []CallRule
	for _, u := range units {
		if u.Source == root+"ssa/rewrite.go" {
			states = append(states, u.State...)
			calls = append(calls, u.Calls...)
		}
	}
	states = append(states, StateRule{Source: "src/cmd/internal/sys/arch.go", Variable: "ArchRISCV64", Field: "archRISCV64"}, StateRule{Source: "src/cmd/internal/sys/arch.go", Variable: "ArchWasm", Field: "archWasm"}, StateRule{Source: root + "types/type.go", Variable: "TypeVoid", Field: "Types.TypeVoid"}, StateRule{Source: "src/internal/buildcfg/exp.go", Variable: "FramePointerEnabled", Field: "FramePointerEnabled"})
	for i := range units {
		u := &units[i]
		if u.Source == "src/cmd/internal/sys/arch.go" {
			*u = Unit{Source: u.Source, WholeFile: true, Context: &ContextRule{Type: "Context", Constructor: "NewContext", Adapter: "adapter_context.go", Selective: true, Fields: map[string]string{}}, Owner: &OwnerRule{Types: []string{"Arch"}, Context: "Context", Field: "compilation"}}
			for _, name := range []string{"Arch386", "ArchAMD64", "ArchARM", "ArchARM64", "ArchLoong64", "ArchMIPS", "ArchMIPSLE", "ArchMIPS64", "ArchMIPS64LE", "ArchPPC64", "ArchPPC64LE", "ArchRISCV64", "ArchS390X", "ArchWasm", "Archs"} {
				u.Context.Fields[name] = name
			}
		}
		if u.Source != root+"ssa/opGen.go" {
			continue
		}
		for _, pair := range [][2]string{{"call", "Block.containsCall"}, {"clobberFlags", "Value.clobbersFlags"}} {
			found := false
			for j := range u.ConstantFields {
				r := &u.ConstantFields[j]
				if r.Variable == "opcodeTable" && r.Field == pair[0] {
					r.Functions = append(r.Functions, pair[1])
					found = true
					break
				}
			}
			if !found {
				u.ConstantFields = append(u.ConstantFields, ConstantFieldRule{Variable: "opcodeTable", Field: pair[0], Functions: []string{pair[1]}})
			}
		}
	}
	for _, file := range []string{"addressingmodes.go", "pair.go", "licm.go", "tighten.go", "tuple.go", "critical.go", "layout.go", "schedule.go", "flagalloc.go", "regalloc.go", "stackalloc.go", "looprotate.go", "trim.go", "nilcheck.go"} {
		u := Unit{Source: root + "ssa/" + file, WholeFile: true, Context: context(), State: states, Calls: calls, ContextFrom: map[string]ContextSource{}}
		entry := func(name string) { u.ContextFrom[name] = ContextSource{Argument: "f", Resolver: "diagnosticContext"} }
		switch file {
		case "addressingmodes.go":
			entry("addressingModes")
			u.Context.Init = true
			u.Context.Fields = map[string]string{"combineFirst": "combineFirst", "combine": "combine", "needSplit": "needSplit"}
		case "pair.go":
			entry("pairLoads")
			entry("pairStores")
			u.Context.Fields = map[string]string{"pairableLoads": "pairableLoads", "pairableStores": "pairableStores"}
		case "licm.go":
			entry("licm")
		case "tighten.go":
			entry("tighten")
		case "schedule.go":
			entry("schedule")
		case "nilcheck.go":
			entry("nilcheckelim2")
			u.Context.Fields = map[string]string{"faultOnLoad": "faultOnLoad"}
		case "stackalloc.go":
			entry("stackalloc")
			u.ContextParams = []string{"stackAllocState.*"}
		case "regalloc.go":
			u.FixedSizeof = []FixedSizeofRule{{Function: "regAllocState.init", Type: "regMask"}}
			entry("regalloc")
			u.Context.Fields = map[string]string{"tmpVal": "tmpVal", "noRegisters": "noRegisters"}
			u.ContextParams = []string{"regAllocState.*", "edgeState.*"}
		}
		replaced := false
		for i := range units {
			if units[i].Source == u.Source {
				units[i] = u
				replaced = true
				break
			}
		}
		if !replaced {
			units = append(units, u)
		}
	}
	return units
}

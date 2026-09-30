package port

// Keep all native generic rewrite and decomposition passes. Their callbacks use
// the same compiler services as architecture lowering; only state ownership is
// changed, never the rewrite or splitting algorithms.
func ssaGenericMigration(units []Unit) []Unit {
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
	states = append(states, StateRule{Source: root + "ir/symtab.go", Variable: "Syms", Field: "IR.Syms"})
	for i := range units {
		u := &units[i]
		switch u.Source {
		case root + "types/type.go":
			u.Declarations = append(u.Declarations, "Type.FieldName")
		case root + "ssa/func.go":
			for _, name := range []string{"SplitString", "SplitInterface", "SplitSlice", "SplitComplex", "SplitInt64", "SplitStruct", "SplitArray", "SplitSlot"} {
				u.Declarations = append(u.Declarations, "Func."+name)
				if name != "SplitStruct" && name != "SplitSlot" {
					u.ContextFrom["Func."+name] = ContextSource{Argument: "f", Resolver: "diagnosticContext"}
				}
			}
			u.State = append(u.State, StateRule{Source: root + "types/type.go", Variable: "Types", Field: "Types.Types"})
			u.Calls = append(u.Calls, CallRule{Source: root + "types/type.go", Function: "NewPtr", Method: "Types.NewPtr"}, CallRule{Source: root + "base/print.go", Function: "FatalfAt", Method: "fatalfAt"})
		}
	}
	for _, file := range []string{"rewritegeneric.go", "rewritedec.go", "rewritedec64.go", "rewritedivisible.go", "rewritedivmod.go", "opt.go", "decompose.go", "softfloat.go", "rewritetern.go", "tern_helpers.go"} {
		u := Unit{Source: root + "ssa/" + file, WholeFile: true, Context: context(), State: states, Calls: calls}
		switch file {
		case "opt.go":
			u.ContextFrom = map[string]ContextSource{"opt": {Argument: "f", Resolver: "diagnosticContext"}, "divisible": {Argument: "f", Resolver: "diagnosticContext"}, "divmod": {Argument: "f", Resolver: "diagnosticContext"}}
		case "decompose.go":
			u.ContextFrom = map[string]ContextSource{"decomposeBuiltin": {Argument: "f", Resolver: "diagnosticContext"}}
		case "softfloat.go":
			u.ContextFrom = map[string]ContextSource{"softfloat": {Argument: "f", Resolver: "diagnosticContext"}}
		case "rewritetern.go":
			u.ReadTables = []string{"truthTableValues"}
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
	units = append(units, Unit{Source: root + "ssa/writebarrier.go", Functions: []string{"isVolatile"}})
	return units
}

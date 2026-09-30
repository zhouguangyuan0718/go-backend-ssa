package port

// Keep the complete native ABI expansion, barrier expansion, and loop-reschedule
// passes. Their temporary names, types, symbols, and experiments come from the
// compiler services already associated with the SSA function.
func ssaCallMigration(units []Unit) []Unit {
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
	states = append(states,
		StateRule{Source: root + "types/type.go", Variable: "TypeResultMem", Field: "Types.TypeResultMem"},
		StateRule{Source: root + "types/fmt.go", Variable: "LocalPkg", Field: "Types.LocalPkg"},
		StateRule{Source: "src/internal/buildcfg/exp.go", Variable: "Experiment", Field: "Types.Experiment"},
	)
	for _, v := range []struct{ source, name, method string }{
		{"types/type.go", "NewPtr", "Types.NewPtr"},
		{"types/type.go", "NewResults", "Types.NewResults"},
		{"types/size.go", "RoundUp", "Types.RoundUp"},
		{"abi/abiutils.go", "RegisterTypes", "ABI.RegisterTypes"},
	} {
		calls = append(calls, CallRule{Source: root + v.source, Function: v.name, Method: v.method})
	}
	for i := range units {
		u := &units[i]
		if u.Source == root+"ssa/func.go" {
			u.Declarations = append(u.Declarations, "Func.NewLocal")
			u.ContextFrom["Func.NewLocal"] = ContextSource{Argument: "f", Resolver: "diagnosticContext"}
			u.Calls = append(u.Calls, CallRule{Source: root + "typecheck/dcl.go", Function: "TempAt", Method: "tempAt"})
		}
	}
	for _, file := range []string{"expand_calls.go", "writebarrier.go", "loopreschedchecks.go"} {
		u := Unit{Source: root + "ssa/" + file, WholeFile: true, Context: context(), State: states, Calls: calls, ContextFrom: map[string]ContextSource{}}
		switch file {
		case "expand_calls.go":
			u.ContextFrom["expandCalls"] = ContextSource{Argument: "f", Resolver: "diagnosticContext"}
			u.ContextParams = []string{"expandState.*", "expandState.Printf", "registerCursor.next"}
		case "writebarrier.go":
			u.ContextFrom["writebarrier"] = ContextSource{Argument: "f", Resolver: "diagnosticContext"}
			u.ContextFrom["Func.computeZeroMap"] = ContextSource{Argument: "f", Resolver: "diagnosticContext"}
			u.ContextFrom["IsNewObject"] = ContextSource{Argument: "v", Resolver: "valueContext"}
		case "loopreschedchecks.go":
			u.ContextFrom["insertLoopReschedChecks"] = ContextSource{Argument: "f", Resolver: "diagnosticContext"}
			u.ContextFrom["findLastMems"] = ContextSource{Argument: "f", Resolver: "diagnosticContext"}
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

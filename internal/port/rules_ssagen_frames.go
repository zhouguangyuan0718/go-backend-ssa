package port

// Preserve frame layout and its complete native local-merging analysis.
func ssagenFrameMigration(units []Unit) []Unit {
	const root = "src/cmd/compile/internal/"
	var frontend Unit
	for i := range units {
		u := &units[i]
		if u.Source == root+"ssa/debug.go" {
			u.Declarations = append(u.Declarations, "FuncDebug")
		}
		if u.Source == root+"ssagen/ssa.go" {
			frontend = *u
		}
	}
	context := &ContextRule{Type: "Context", Constructor: "newContext", Adapter: "adapter_context.go", Selective: true, RedirectTrace: true}
	state := []StateRule{
		{Source: root + "base/link.go", Variable: "Ctxt", Field: "SSA.IR.Base.Ctxt"},
		{Source: root + "base/debug.go", Variable: "Debug", Field: "SSA.IR.Debug"},
		{Source: root + "base/flag.go", Variable: "Flag", Field: "SSA.IR.Base.Flag"},
		{Source: root + "base/hashdebug.go", Variable: "MergeLocalsHash", Field: "SSA.IR.Base.MergeLocalsHash"},
		{Source: root + "ir/symtab.go", Variable: "Syms", Field: "SSA.IR.Syms"},
		{Source: root + "types/size.go", Variable: "PtrSize", Field: "ptrSize"},
	}
	calls := []CallRule{{Source: root + "base/print.go", Function: "Fatalf", Method: "fatalf"}, {Source: root + "base/print.go", Function: "FatalfAt", Method: "fatalfAt"}}
	for i := range units {
		if units[i].Source == root+"liveness/plive.go" {
			units[i] = Unit{Source: root + "liveness/plive.go", Context: context, State: state, Calls: calls,
				Owner:        &OwnerRule{Types: []string{"Liveness"}, Context: "Context", Field: "compilation"},
				Declarations: []string{"Map", "Map.reset", "Map.set", "Map.setUnsafeVal", "Map.setUnsafeBlock", "Map.Get", "Map.GetUnsafe", "Map.GetUnsafeBlock", "blockEffects", "Liveness", "progeffectscache", "liveEffect", "uevar", "livenessFuncCache", "Liveness.initcache", "Liveness.valueEffects", "Liveness.blockEffects", "Liveness.markUnsafePoints", "Liveness.prologue", "Liveness.solve", "Liveness.enableClobber"},
				Functions:    []string{"affectedVar", "newliveness", "IsUnsafe", "isfat"},
				HostCalls:    []HostCallRule{{Package: "os", Function: "Getenv", Method: "clobberDeadHash", StringArgument: "GOCLOBBERDEADHASH"}},
			}
		}
	}
	// Keep liveness ahead of ssagen so its native types and methods are available
	// when validating the frame adapter's cross-package signatures.
	var retained, backend []Unit
	for _, u := range units {
		if u.sourcePackage() == "cmd/compile/internal/ssagen" {
			backend = append(backend, u)
		} else {
			retained = append(retained, u)
		}
	}
	units = retained
	units = append(units,
		Unit{Source: root + "liveness/mergelocals.go", WholeFile: true, Context: context, State: state, Calls: calls, Owner: &OwnerRule{Types: []string{"cstate"}, Context: "Context", Field: "compilation"}, Checks: map[string]string{"MergeLocals": "checkMerge"}, Literals: []LiteralRule{{Source: root + "liveness/intervals.go", Type: "IntervalsBuilder", Method: "bindIntervalBuilders", MakeSlice: true}}},
		Unit{Source: root + "liveness/intervals.go", WholeFile: true, Context: context, Owner: &OwnerRule{Types: []string{"IntervalsBuilder"}, Context: "Context", Field: "compilation"}},
		Unit{Source: root + "liveness/bvset.go", WholeFile: true},
	)
	units = append(units, backend...)
	frameContext := *frontend.Context
	frameContext.Fields = nil
	frameState := append([]StateRule{}, frontend.State...)
	frameState = append(frameState,
		StateRule{Source: root + "types/size.go", Variable: "RegSize", Field: "regSize"},
		StateRule{Source: root + "types/size.go", Variable: "PtrSize", Field: "ptrSize"},
		StateRule{Source: root + "types/size.go", Variable: "MaxWidth", Field: "maxWidth"},
		StateRule{Source: "src/internal/buildcfg/exp.go", Variable: "FramePointerEnabled", Field: "SSA.FramePointerEnabled"},
	)
	frameCalls := append([]CallRule{}, frontend.Calls...)
	frameCalls = append(frameCalls, CallRule{Source: root + "types/size.go", Function: "RoundUp", Method: "Typecheck.IR.Types.RoundUp"})
	frameCalls = append(frameCalls, CallRule{Source: root + "base/print.go", Function: "FatalfAt", Method: "fatalfAt"}, CallRule{Source: root + "liveness/mergelocals.go", Function: "MergeLocals", Method: "Liveness.MergeLocals"})
	units = append(units, Unit{Source: root + "ssagen/pgen.go", Context: &frameContext, State: frameState, Calls: frameCalls, Functions: []string{"cmpstackvarlt", "needAlloc", "StackOffset"}, Declarations: []string{"ssafn.AllocFrame"}, Checks: map[string]string{"ssafn.AllocFrame": "checkFrame"}})
	return units
}

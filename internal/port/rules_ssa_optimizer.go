package port

// Preserve native optimization algorithms, with compilation state derived from
// the graph. Only machine-independent nil-check elimination is selected here;
// the scheduled variant belongs to the later scheduling and allocation closure.
func ssaOptimizerMigration(units []Unit) []Unit {
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
	for _, n := range []string{"HasDebugHash", "DebugHashMatchPkgFunc"} {
		calls = append(calls, CallRule{Source: root + "base/hashdebug.go", Function: n, Method: "IR.Base." + n})
	}
	for i := range units {
		u := &units[i]
		switch u.Source {
		case root + "ssa/compile.go":
			// Only checkEnabled is in this closure. Native init validates the
			// unselected passes/passOrder tables and does not set this flag.
			u.Context = context()
			u.Context.DeferInit = true
			u.Context.Fields = map[string]string{"checkEnabled": "checkEnabled"}
		case root + "ssa/func.go":
			u.Declarations = append(u.Declarations, "Func.DebugHashMatch", "Func.newPoset", "Func.retPoset")
			u.ContextFrom["Func.DebugHashMatch"] = ContextSource{Argument: "f", Resolver: "diagnosticContext"}
			u.Calls = append(u.Calls, calls[len(calls)-2:]...)
		case root + "ssa/opGen.go":
			for _, v := range []struct {
				field     string
				functions []string
			}{
				{"commutative", []string{"cse"}}, {"argLen", []string{"zcse"}},
				{"nilCheck", []string{"isEmpty"}}, {"hasSideEffects", []string{"canSpeculativelyExecute"}},
			} {
				found := false
				for j := range u.ConstantFields {
					if u.ConstantFields[j].Variable == "opcodeTable" && u.ConstantFields[j].Field == v.field {
						u.ConstantFields[j].Functions = append(u.ConstantFields[j].Functions, v.functions...)
						found = true
						break
					}
				}
				if !found {
					u.ConstantFields = append(u.ConstantFields, ConstantFieldRule{Variable: "opcodeTable", Field: v.field, Functions: v.functions})
				}
			}
		}
	}
	for _, file := range []string{"branchelim.go", "checkbce.go", "cse.go", "deadstore.go", "fuse.go", "fuse_branchredirect.go", "fuse_comparisons.go", "known_bits.go", "memcombine.go", "nilcheck.go", "phiopt.go", "sccp.go", "shortcircuit.go", "uses.go", "zcse.go", "cpufeatures.go", "lca.go", "prove.go", "poset.go", "loopbce.go", "downward_counting_loop.go"} {
		u := Unit{Source: root + "ssa/" + file, WholeFile: true, Context: context(), State: states, Calls: calls}
		u.ContextFrom = map[string]ContextSource{}
		switch file {
		case "fuse_branchredirect.go":
			u.ContextFrom["fuseBranchRedirect"] = ContextSource{Argument: "f", Resolver: "diagnosticContext"}
		case "cse.go":
			u.ContextFrom["cse"] = ContextSource{Argument: "f", Resolver: "diagnosticContext"}
		case "checkbce.go":
			u.ContextFrom["checkbce"] = ContextSource{Argument: "f", Resolver: "diagnosticContext"}
		case "memcombine.go":
			u.ContextFrom["memcombine"] = ContextSource{Argument: "f", Resolver: "diagnosticContext"}
		case "sccp.go":
			u.ContextFrom["computeLattice"] = ContextSource{Argument: "f", Resolver: "diagnosticContext"}
		case "nilcheck.go":
			u.WholeFile = false
			u.Functions = []string{"nilcheckelim"}

		case "loopbce.go":
			u.ContextFrom["findIndVar"] = ContextSource{Argument: "f", Resolver: "diagnosticContext"}
		case "prove.go":
			u.ReadTables = []string{"relationStrings", "domainStrings", "opMin", "opMax", "opUMax", "reverseBits", "ctzNonZeroOp", "mostNegativeDividend", "unsignedOp", "bytesizeToConst", "bytesizeToNeq", "bytesizeToAnd", "invertEqNeqOp"}
			u.StatelessFunctions = []string{"convertIntWithBitsize"}
			u.Context.Fields = map[string]string{"domainRelationTable": "domainRelationTable", "checkpointBound": "checkpointBound"}
			u.ContextParams = []string{"factsTable.*"}
			u.ContextFrom["prove"] = ContextSource{Argument: "f", Resolver: "diagnosticContext"}
		case "poset.go":
			u.WholeFile = false
			u.Functions = []string{"computeBitsetSize", "newBitset", "newedge", "newPoset"}
			u.Declarations = []string{"Cache.allocBitset", "Cache.freeBitset", "bitset.Reset", "bitset.Set", "bitset.Clear", "bitset.Test", "posetEdge.Target", "posetEdge.Strict", "posetEdge.String", "poset.SetUnsigned", "poset.setchl", "poset.setchr", "poset.chl", "poset.chr", "poset.children", "poset.upush", "poset.upushnew", "poset.upushneq", "poset.upushalias", "poset.addchild", "poset.newnode", "poset.lookup", "poset.aliasnewnode", "poset.aliasnodes", "poset.isroot", "poset.changeroot", "poset.removeroot", "poset.dfs", "poset.reaches", "poset.findroot", "poset.mergeroot", "poset.collapsepath", "poset.findpaths", "poset.findpaths1", "poset.isnoneq", "poset.setnoneq", "poset.CheckIntegrity", "poset.CheckEmpty", "poset.Ordered", "poset.OrderedOrEqual", "poset.Equal", "poset.NonEqual", "poset.setOrder", "poset.SetOrder", "poset.SetOrderOrEqual", "poset.SetEqual", "poset.SetNonEqual", "poset.Checkpoint", "poset.Undo", "bitset", "undoType", "posetUndo", "posetEdge", "posetNode", "poset", "uintSize", "undoInvalid", "posetFlagUnsigned"}
			u.Context.Fields = map[string]string{"debugPoset": "debugPoset"}
			u.ContextParams = []string{"poset.*", "poset.Ordered", "poset.OrderedOrEqual", "poset.Equal", "poset.NonEqual", "poset.SetOrder", "poset.SetOrderOrEqual", "poset.SetEqual", "poset.SetNonEqual", "poset.Undo"}

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
	units = append(units, Unit{Source: root + "ssa/schedule.go", Functions: []string{"storeOrder", "valuePosCmp"}})
	return units
}

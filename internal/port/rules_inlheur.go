package port

func inlheurMigration() []Unit {
	const root = "src/cmd/compile/internal/inline/inlheur/"
	units := []Unit{
		{Source: root + "analyze.go", Declarations: []string{"debugTraceFuncs", "propAnalyzer", "fnInlHeur", "csDelimiter", "preambleDelimiter", "fnDelimiter", "comDelimiter"}, Functions: []string{"DumpFuncProps", "captureFuncDumpEntry", "dumpFilePreamble", "dumpFnPreamble", "sortFnInlHeurSlice", "AnalyzeFunc", "TearDown", "analyzeFunc", "revisitInlinability", "computeFuncProps", "runAnalyzersOnFunction", "propsForFunc", "fnFileLine", "Enabled", "UnitTesting"}},
	}
	for _, name := range []string{"analyze_func_callsites.go", "analyze_func_flags.go", "analyze_func_params.go", "analyze_func_returns.go", "callsite.go", "eclassify.go", "function_properties.go", "names.go", "score_callresult_uses.go", "scoring.go", "serialize.go", "trace_off.go", "actualexprpropbits_string.go", "cspropbits_string.go", "funcprop_string.go", "funcpropbits_string.go", "parampropbits_string.go", "pstate_string.go", "resultpropbits_string.go", "scoreadjusttyp_string.go"} {
		units = append(units, Unit{Source: root + name, WholeFile: true})
	}
	owners := map[string][]string{
		"analyze_func_callsites.go": {"callSiteAnalyzer", "callSiteTableBuilder"},
		"analyze_func_flags.go":     {"funcFlagsAnalyzer"},
		"analyze_func_params.go":    {"paramsAnalyzer", "condLevelTracker"},
		"analyze_func_returns.go":   {"resultsAnalyzer"},
		"callsite.go":               {"CallSite"}, "eclassify.go": {"exprClassifier"}, "names.go": {"nameFinder"}, "score_callresult_uses.go": {"resultUseAnalyzer"},
	}
	tables := map[string][]string{
		"actualexprpropbits_string.go": {"_ActualExprPropBits_value", "_ActualExprPropBits_index"},
		"cspropbits_string.go":         {"_CSPropBits_value", "_CSPropBits_index"},
		"funcpropbits_string.go":       {"_FuncPropBits_value", "_FuncPropBits_index"},
		"parampropbits_string.go":      {"_ParamPropBits_value", "_ParamPropBits_index"},
		"pstate_string.go":             {"_pstate_index"},
		"resultpropbits_string.go":     {"_ResultPropBits_value", "_ResultPropBits_index"},
		"scoreadjusttyp_string.go":     {"_scoreAdjustTyp_value", "_scoreAdjustTyp_index"},
	}
	for i := range units {
		u := &units[i]
		name := u.Source[len(root):]
		u.Context = &ContextRule{Type: "Context", Constructor: "newContext", Selective: true, PreserveNilArgs: true, RedirectTrace: true, Adapter: "adapter_context.go"}
		if names := owners[name]; len(names) > 0 {
			u.Owner = &OwnerRule{Types: names, Context: "Context", Field: "compilation"}
		}
		u.ReadTables = tables[name]
		if name == "callsite.go" {
			u.ContextParams = []string{"CallSiteTab.merge"}
		}
		if name == "analyze.go" {
			u.Checks = map[string]string{"AnalyzeFunc": "checkAnalyze", "DumpFuncProps": "checkDumpFunc"}
			u.Context.Fields = map[string]string{"fpmap": "fpmap", "dumpBuffer": "dumpBuffer"}
		}
		if name == "scoring.go" {
			u.Checks = map[string]string{"ScoreCalls": "checkFunc", "GetCallSiteScore": "checkCallScore", "LargestNegativeScoreAdjustment": "checkProps"}
			u.Context.Fields = map[string]string{}
			for _, n := range []string{"adjValues", "mayMustAdj", "resultFlagToPositiveAdj", "paramFlagToPositiveAdj", "callSiteTab", "scoreCallsCache", "allCallSites"} {
				u.Context.Fields[n] = n
			}
		}
	}
	units[0].State = []StateRule{
		{Source: "src/cmd/compile/internal/base/link.go", Variable: "Ctxt", Field: "Typecheck.IR.Base.Ctxt"},
		{Source: "src/cmd/compile/internal/base/debug.go", Variable: "Debug", Field: "Typecheck.IR.Debug"},
		{Source: "src/internal/buildcfg/exp.go", Variable: "Experiment", Field: "Typecheck.IR.Types.Experiment"},
		{Source: "src/cmd/compile/internal/types/fmt.go", Variable: "LocalPkg", Field: "Typecheck.IR.Types.LocalPkg"},
		{Source: "src/cmd/compile/internal/typecheck/iimport.go", Variable: "HaveInlineBody", Field: "Typecheck.HaveInlineBody"},
	}
	units[0].Calls = []CallRule{
		{Source: root + "analyze.go", Function: "emitDumpToFile", Method: "emitDump"},
		{Source: "src/cmd/compile/internal/base/print.go", Function: "Fatalf", Method: "fatalf"},
		{Source: "src/cmd/compile/internal/base/print.go", Function: "FatalfAt", Method: "fatalfAt"},
		{Source: "src/cmd/compile/internal/ir/expr.go", Function: "StaticValue", Method: "Typecheck.IR.StaticValue"},
		{Source: "src/cmd/compile/internal/ir/expr.go", Function: "StaticCalleeName", Method: "Typecheck.IR.StaticCalleeName"},
		{Source: "src/cmd/compile/internal/ir/expr.go", Function: "Reassigned", Method: "Typecheck.IR.Reassigned"},
		{Source: "src/cmd/compile/internal/ir/node.go", Function: "Line", Method: "Typecheck.IR.Line"},
		{Source: "src/cmd/compile/internal/pgoir/irgraph.go", Function: "DirectCallee", Method: "PGO.DirectCallee"},
	}
	return units
}

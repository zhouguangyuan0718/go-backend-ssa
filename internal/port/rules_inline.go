package port

import (
	"sort"
	"strings"
)

func inlineMigration() []Unit {
	const root = "src/cmd/compile/internal/"
	u := Unit{Source: root + "inline/inl.go",
		Declarations: []string{"inlineMaxBudget", "hairyVisitor", "hairyVisitor.tooHairy", "hairyVisitor.doNode"},
		Functions:    []string{"IsPgoHotFunc", "HasPgoHotInline", "PGOInlinePrologue", "hotNodesFromCDF", "CanInlineFuncs", "simdCreditMultiplier", "inlineBudget", "CanInline", "noteInlinableFunc", "InlineImpossible", "canDelayResults", "isDebugFn", "IsBigFunc", "inlineCallCheck", "InlineCallTarget", "inlCallee", "inlineCostOK", "parsePos", "canInlineCallExpr", "pruneUnusedAutos", "doList", "isIndexingCoverageCounter", "isAtomicCoverageCounterUpdate", "PostProcessCallSites", "analyzeFuncProps"},
		Owner:        &OwnerRule{Types: []string{"hairyVisitor"}, Context: "Context", Field: "compilation"},
		Context:      &ContextRule{Type: "Context", Constructor: "newContext", Selective: true, PreserveNilArgs: true, RedirectTrace: true, Adapter: "adapter_context.go", Fields: map[string]string{}},
		Checks:       map[string]string{"CanInline": "checkFuncProfile", "CanInlineFuncs": "checkFuncsProfile", "IsPgoHotFunc": "checkFuncProfile", "HasPgoHotInline": "checkFunc", "PGOInlinePrologue": "checkProfile", "InlineCallTarget": "checkCallProfile", "InlineImpossible": "checkFunc", "PostProcessCallSites": "checkProfile"},
	}
	for _, n := range []string{"candHotCalleeMap", "hasHotCall", "candHotEdgeMap", "inlineHotCallSiteThresholdPercent", "inlineCDFHotCallSiteThresholdPercent", "inlineHotMaxBudget"} {
		u.Context.Fields[n] = n
	}
	u.State = []StateRule{
		{Source: root + "base/link.go", Variable: "Ctxt", Field: "Typecheck.IR.Base.Ctxt"},
		{Source: root + "base/flag.go", Variable: "Flag", Field: "Typecheck.IR.Base.Flag"},
		{Source: root + "base/debug.go", Variable: "Debug", Field: "Typecheck.IR.Debug"},
		{Source: root + "base/hashdebug.go", Variable: "PGOHash", Field: "Typecheck.IR.Base.PGOHash"},
		{Source: root + "typecheck/iimport.go", Variable: "HaveInlineBody", Field: "Typecheck.HaveInlineBody"},
		{Source: root + "ir/expr.go", Variable: "IsIntrinsicCall", Field: "Typecheck.IR.IsIntrinsicCall"},
	}
	calls := map[string][]string{
		"base/print.go":             {"Fatalf", "FatalfAt"},
		"ir/scc.go":                 {"VisitFuncsBottomUp"},
		"ir/expr.go":                {"MethodExprName", "StaticValue", "StaticCalleeName"},
		"ir/node.go":                {"Line"},
		"ir/val.go":                 {"BoolVal", "Int64Val"},
		"ir/dump.go":                {"MatchAstDump", "AstDump"},
		"types/identity.go":         {"Identical"},
		"types/type.go":             {"RuntimeSymName", "ReflectSymName", "IsNoInstrumentPkg", "IsNoRacePkg", "IsRuntimePkg"},
		"inline/inlheur/analyze.go": {"Enabled", "AnalyzeFunc"},
		"inline/inlheur/scoring.go": {"BudgetExpansion", "GetCallSiteScore", "DumpInlCallSiteScores"},
		"logopt/log_opts.go":        {"Enabled", "LogOpt"},
		"pgoir/irgraph.go":          {"NodeLineOffset"},
	}
	files := make([]string, 0, len(calls))
	for f := range calls {
		files = append(files, f)
	}
	sort.Strings(files)
	for _, file := range files {
		names := calls[file]
		for _, n := range names {
			method := ""
			switch file[:strings.IndexByte(file, '/')] {
			case "base":
				if n == "Fatalf" {
					method = "fatalf"
				} else {
					method = "fatalfAt"
				}
			case "ir":
				method = "Typecheck.IR." + n
			case "types":
				method = "Typecheck.IR.Types." + n
			case "inline":
				method = "Heuristics." + n
			case "logopt":
				method = "Log." + n
			case "pgoir":
				method = "PGO." + n
			}
			u.Calls = append(u.Calls, CallRule{Source: root + file, Function: n, Method: method})
		}
	}
	return []Unit{u}
}

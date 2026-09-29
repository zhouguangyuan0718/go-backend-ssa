package port

import (
	"sort"
	"strings"
)

func interleavedMigration() []Unit {
	const root = "src/cmd/compile/internal/"
	u := Unit{Source: root + "inline/interleaved/interleaved.go", WholeFile: true,
		Context: &ContextRule{Type: "Context", Constructor: "newContext", Selective: true, PreserveNilArgs: true, RedirectTrace: true, Adapter: "adapter_context.go"},
		Owner:   &OwnerRule{Types: []string{"inlClosureState"}, Context: "Context", Field: "compilation"},
		Checks:  map[string]string{"DevirtualizeAndInlinePackage": "checkPackage", "DevirtualizeAndInlineFunc": "checkFunc"},
		State: []StateRule{
			{Source: root + "base/flag.go", Variable: "Flag", Field: "Inline.Typecheck.IR.Base.Flag"},
			{Source: root + "base/debug.go", Variable: "Debug", Field: "Inline.Typecheck.IR.Debug"},
			{Source: root + "ir/func.go", Variable: "CurFunc", Field: "Inline.Typecheck.IR.CurFunc"},
			{Source: root + "typecheck/target.go", Variable: "Target", Field: "Inline.Typecheck.Target"},
		}}
	calls := map[string][]string{
		"ir/fmt.go": {"DumpList"}, "ir/scc.go": {"VisitFuncsBottomUp"}, "ir/expr.go": {"NewParenExpr"}, "ir/func.go": {"WithFunc"}, "ir/node.go": {"Line"},
		"inline/inl.go":                {"CanInlineFuncs", "InlineCallTarget", "TryInlineCall", "PostProcessCallSites"},
		"devirtualize/devirtualize.go": {"StaticCall"}, "devirtualize/pgo.go": {"ProfileGuided"},
		"inline/inlheur/analyze.go": {"Enabled", "DumpFuncProps", "TearDown"}, "inline/inlheur/scoring.go": {"SetupScoreAdjustments", "ScoreCalls", "ScoreCallsCleanup"},
	}
	files := make([]string, 0, len(calls))
	for f := range calls {
		files = append(files, f)
	}
	sort.Strings(files)
	for _, file := range files {
		for _, n := range calls[file] {
			prefix := "Inline."
			if strings.HasPrefix(file, "ir/") {
				prefix = "Inline.Typecheck.IR."
			} else if strings.HasPrefix(file, "devirtualize/") {
				prefix = "Devirtualize."
			} else if strings.HasPrefix(file, "inline/inlheur/") {
				prefix = "Inline.Heuristics."
			}
			u.Calls = append(u.Calls, CallRule{Source: root + file, Function: n, Method: prefix + n})
		}
	}
	return []Unit{u}
}

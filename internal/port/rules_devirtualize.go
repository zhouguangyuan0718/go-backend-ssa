package port

import (
	"sort"
	"strings"
)

func devirtualizeMigration() []Unit {
	const root = "src/cmd/compile/internal/"
	units := []Unit{
		{Source: root + "devirtualize/devirtualize.go", WholeFile: true,
			Owner:          &OwnerRule{Types: []string{"State"}, Context: "Context", Field: "compilation"},
			ContextMethods: map[string]string{"StaticCall": "staticCall"},
			Checks:         map[string]string{"State.InlinedCall": "checkInlinedCall"}},
		{Source: root + "devirtualize/pgo.go", WholeFile: true, Checks: map[string]string{"ProfileGuided": "checkProfileGuided"}},
	}
	for i := range units {
		units[i].Context = &ContextRule{Type: "Context", Constructor: "newContext", Selective: true, PreserveNilArgs: true, RedirectTrace: true, Adapter: "adapter_context.go"}
	}
	u := &units[0]
	u.Context.Fields = map[string]string{"noType": "noType"}
	u.State = []StateRule{
		{Source: root + "base/link.go", Variable: "Ctxt", Field: "Typecheck.IR.Base.Ctxt"},
		{Source: root + "base/flag.go", Variable: "Flag", Field: "Typecheck.IR.Base.Flag"},
		{Source: root + "base/debug.go", Variable: "Debug", Field: "Typecheck.IR.Debug"},
		{Source: root + "base/hashdebug.go", Variable: "PGOHash", Field: "Typecheck.IR.Base.PGOHash"},
		{Source: root + "base/print.go", Variable: "Pos", Field: "Typecheck.IR.Types.Pos"},
		{Source: root + "ir/func.go", Variable: "CurFunc", Field: "Typecheck.IR.CurFunc"},
		{Source: root + "types/type.go", Variable: "Types", Field: "Typecheck.IR.Types.Types"},
	}
	calls := map[string][]string{
		"base/print.go": {"Fatalf", "FatalfAt", "Warn", "WarnfAt", "Assert", "AssertfAt"},
		"ir/stmt.go":    {"NewAssignListStmt", "NewIfStmt"},
		"ir/expr.go":    {"NewBinaryExpr", "NewConvExpr", "NewInlinedCallExpr", "NewTypeAssertExpr", "StaticValue"},
		"ir/func.go":    {"FuncPC"}, "ir/node.go": {"Line", "OuterValue"},
		"typecheck/typecheck.go": {"Expr", "Stmt", "Call"}, "typecheck/dcl.go": {"TempAt"},
		"typecheck/subr.go": {"Implements", "ImplementsExplain"}, "typecheck/expr.go": {"XDotMethod"}, "typecheck/func.go": {"FixMethodCall"},
		"types/size.go": {"CheckSize"}, "types/identity.go": {"Identical"},
		"inline/inl.go": {"InlineImpossible"}, "pgoir/irgraph.go": {"NodeLineOffset", "DirectCallee"}, "logopt/log_opts.go": {"LogOpt", "Enabled"},
	}
	files := make([]string, 0, len(calls))
	for f := range calls {
		files = append(files, f)
	}
	sort.Strings(files)
	for _, file := range files {
		for _, n := range calls[file] {
			prefix := map[string]string{"ir": "Typecheck.IR.", "typecheck": "Typecheck.", "types": "Typecheck.IR.Types.", "inline": "Inline.", "pgoir": "Inline.PGO.", "logopt": "Inline.Log."}[strings.SplitN(file, "/", 2)[0]]
			method := prefix + n
			if strings.HasPrefix(file, "base/") {
				method = map[string]string{"Fatalf": "fatalf", "FatalfAt": "fatalfAt", "Warn": "warn", "WarnfAt": "warnfAt", "Assert": "assertCondition", "AssertfAt": "assertfAt"}[n]
			}
			u.Calls = append(u.Calls, CallRule{Source: root + file, Function: n, Method: method})
		}
	}
	return units
}

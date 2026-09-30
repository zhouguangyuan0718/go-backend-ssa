package port

import (
	"sort"
	"strings"
)

func escapeMigration() []Unit {
	const root = "src/cmd/compile/internal/"
	receivers := map[string][]string{
		"alias.go": {"aliasAnalysis.*"}, "assign.go": {"escape.*"}, "call.go": {"escape.*"},
		"escape.go": {"batch.*"}, "expr.go": {"escape.*"}, "graph.go": {"batch.*", "escape.*", "location.*", "hole.*"},
		"leaks.go": {"leaks.*", "leaks.AddHeap", "leaks.AddMutator", "leaks.AddCallee", "leaks.AddResult", "leaks.Optimize", "leaks.Encode"},
		"solve.go": {"batch.*", "queue.*"}, "stmt.go": {"escape.*"}, "utils.go": nil,
	}
	var units []Unit
	for _, file := range []string{"alias.go", "assign.go", "call.go", "escape.go", "expr.go", "graph.go", "leaks.go", "solve.go", "stmt.go", "utils.go"} {
		units = append(units, Unit{Source: root + "escape/" + file, WholeFile: true, Context: &ContextRule{Type: "Context", Constructor: "newContext", Selective: true, PreserveNilArgs: true, RedirectTrace: true, Adapter: "adapter_context.go"}, ContextParams: receivers[file]})
	}
	units[3].Checks = map[string]string{"Funcs": "checkFuncs"}
	// Native Batch is driven only by Funcs; its oracle cache remains invocation-local.
	units[3].ContextMethods = map[string]string{"Batch": "batch"}
	units[6].Context.Fields = map[string]string{"leakTagCache": "leakTagCache"}
	units[9].Checks = map[string]string{"HeapAllocReason": "checkOptionalNode"}
	u := &units[0]
	u.State = []StateRule{
		{Source: root + "base/link.go", Variable: "Ctxt", Field: "Typecheck.IR.Base.Ctxt"},
		{Source: root + "base/flag.go", Variable: "Flag", Field: "Typecheck.IR.Base.Flag"},
		{Source: root + "base/debug.go", Variable: "Debug", Field: "Typecheck.IR.Debug"},
		{Source: root + "base/print.go", Variable: "Pos", Field: "Typecheck.IR.Types.Pos"},
		{Source: root + "base/hashdebug.go", Variable: "LiteralAllocHash", Field: "Typecheck.IR.Base.LiteralAllocHash"},
		{Source: root + "types/type.go", Variable: "Types", Field: "Typecheck.IR.Types.Types"},
		{Source: root + "types/size.go", Variable: "PtrSize", Field: "ptrSize"},
		{Source: root + "ir/cfg.go", Variable: "MaxStackVarSize", Field: "Typecheck.IR.MaxStackVarSize"},
		{Source: root + "ir/cfg.go", Variable: "MaxImplicitStackVarSize", Field: "Typecheck.IR.MaxImplicitStackVarSize"},
		{Source: "src/internal/goexperiment/exp_runtimefreegc_off.go", Variable: "RuntimeFreegc", Field: "Typecheck.IR.Types.Experiment.RuntimeFreegc"},
	}
	calls := map[string][]string{
		"base/print.go": {"Fatalf", "FatalfAt", "ErrorfAt", "WarnfAt", "Assert", "FmtPos"},
		"ir/dump.go":    {"MatchAstDump", "AstDump"}, "ir/fmt.go": {"Dump"},
		"ir/expr.go": {"NewBasicLit", "ShouldCheckPtr", "ShouldAsanCheckPtr", "IsReflectHeaderDataField", "StaticCalleeName", "SameSafeExpr"},
		"ir/stmt.go": {"NewAssignStmt", "NewDecl"}, "ir/node.go": {"SetPos", "HasUniquePos"},
		"ir/scc.go": {"VisitFuncsBottomUp"}, "ir/val.go": {"Int64Val", "ValidTypeForConst"},
		"typecheck/typecheck.go": {"Stmt", "Stmts", "Expr"}, "typecheck/dcl.go": {"TempAt"},
		"typecheck/func.go": {"AssertFixedCall", "ClosureType", "MethodValueType"},
		"types/type.go":     {"IsDirectIface"}, "logopt/log_opts.go": {"Enabled", "LogOpt"},
	}
	files := make([]string, 0, len(calls))
	for f := range calls {
		files = append(files, f)
	}
	sort.Strings(files)
	for _, file := range files {
		for _, name := range calls[file] {
			prefix := map[string]string{"ir": "Typecheck.IR.", "typecheck": "Typecheck.", "types": "Typecheck.IR.Types.", "logopt": "Log."}[strings.SplitN(file, "/", 2)[0]]
			method := prefix + name
			if strings.HasPrefix(file, "base/") {
				method = map[string]string{"Fatalf": "fatalf", "FatalfAt": "fatalfAt", "ErrorfAt": "errorfAt", "WarnfAt": "warnfAt", "Assert": "assertCondition", "FmtPos": "Typecheck.IR.Base.FmtPos"}[name]
			}
			u.Calls = append(u.Calls, CallRule{Source: root + file, Function: name, Method: method})
		}
	}
	return units
}

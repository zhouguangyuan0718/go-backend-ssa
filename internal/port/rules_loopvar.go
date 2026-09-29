package port

import (
	"sort"
	"strings"
)

func loopvarMigration() []Unit {
	const root = "src/cmd/compile/internal/"
	u := Unit{Source: root + "loopvar/loopvar.go", WholeFile: true,
		Context: &ContextRule{Type: "Context", Constructor: "newContext", Selective: true, PreserveNilArgs: true, Adapter: "adapter_context.go"},
		Checks:  map[string]string{"ForCapture": "checkFunc", "LogTransformations": "checkTransformations"},
		State: []StateRule{
			{Source: root + "base/link.go", Variable: "Ctxt", Field: "Typecheck.IR.Base.Ctxt"},
			{Source: root + "base/print.go", Variable: "Pos", Field: "Typecheck.IR.Types.Pos"},
			{Source: root + "base/debug.go", Variable: "Debug", Field: "Typecheck.IR.Debug"},
			{Source: root + "base/hashdebug.go", Variable: "LoopVarHash", Field: "Typecheck.IR.Base.LoopVarHash"},
			{Source: root + "types/type.go", Variable: "Types", Field: "Typecheck.IR.Types.Types"},
		}}
	calls := map[string][]string{
		"base/print.go": {"Fatalf", "WarnfAt"},
		"ir/node.go":    {"OuterValue"}, "ir/func.go": {"WithFunc"}, "ir/dump.go": {"MatchAstDump", "AstDump"},
		"ir/stmt.go":  {"NewAssignStmt", "NewLabelStmt", "NewIfStmt", "NewDecl", "NewBranchStmt"},
		"ir/const.go": {"NewBool"}, "ir/expr.go": {"NewUnaryExpr"},
		"typecheck/typecheck.go": {"Stmt"}, "typecheck/dcl.go": {"TempAt"}, "typecheck/syms.go": {"Lookup"},
		"logopt/log_opts.go": {"Enabled", "LogOpt", "LogOptRange"},
	}
	files := make([]string, 0, len(calls))
	for file := range calls {
		files = append(files, file)
	}
	sort.Strings(files)
	for _, file := range files {
		for _, n := range calls[file] {
			prefix := map[string]string{"ir": "Typecheck.IR.", "typecheck": "Typecheck.", "logopt": "Log."}[strings.SplitN(file, "/", 2)[0]]
			method := prefix + n
			if strings.HasPrefix(file, "base/") {
				method = map[string]string{"Fatalf": "fatalf", "WarnfAt": "warnfAt"}[n]
			}
			u.Calls = append(u.Calls, CallRule{Source: root + file, Function: n, Method: method})
		}
	}
	return []Unit{u}
}

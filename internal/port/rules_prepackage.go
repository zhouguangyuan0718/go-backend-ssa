package port

import (
	"sort"
	"strings"
)

// These complete native passes run after unified IR and before inlining.
func prepackageMigration() []Unit {
	const root = "src/cmd/compile/internal/"
	bloop := Unit{Source: root + "bloop/bloop.go", WholeFile: true,
		Context: &ContextRule{Type: "Context", Constructor: "newContext", Selective: true, PreserveNilArgs: true, Adapter: "adapter_context.go"},
		Owner:   &OwnerRule{Types: []string{"editor"}, Context: "Context", Field: "compilation"},
		Checks:  map[string]string{"Walk": "checkPackage"},
		State:   []StateRule{{Source: root + "base/flag.go", Variable: "Flag", Field: "Typecheck.IR.Base.Flag"}, {Source: root + "types/type.go", Variable: "Types", Field: "Typecheck.IR.Types.Types"}},
	}
	cover := Unit{Source: root + "coverage/cover.go", WholeFile: true,
		Context: &ContextRule{Type: "Context", Constructor: "newContext", Selective: true, PreserveNilArgs: true, Adapter: "adapter_context.go"},
		Checks:  map[string]string{"Fixup": "checkTarget"},
		State: []StateRule{
			{Source: root + "base/flag.go", Variable: "Flag", Field: "Typecheck.IR.Base.Flag"},
			{Source: root + "base/link.go", Variable: "Ctxt", Field: "Typecheck.IR.Base.Ctxt"},
			{Source: root + "base/print.go", Variable: "Pos", Field: "Typecheck.IR.Types.Pos"},
			{Source: root + "typecheck/target.go", Variable: "Target", Field: "Typecheck.Target"},
			{Source: root + "types/type.go", Variable: "Types", Field: "Typecheck.IR.Types.Types"},
		},
	}
	calls := []map[string][]string{
		{"base/print.go": {"FatalfAt", "WarnfAt"}, "ir/expr.go": {"NewConvExpr", "MethodExprName"}, "ir/stmt.go": {"NewBlockStmt", "NewAssignListStmt", "NewAssignStmt", "NewDecl"}, "ir/dump.go": {"MatchAstDump", "AstDump"}, "typecheck/typecheck.go": {"Call", "AssignExpr", "Stmt"}, "typecheck/subr.go": {"NodAddr"}, "typecheck/syms.go": {"LookupRuntime"}, "typecheck/dcl.go": {"TempAt"}},
		{"base/print.go": {"Fatalf"}, "ir/const.go": {"NewInt", "NewString", "NewBool"}, "ir/expr.go": {"NewCompLitExpr"}, "ir/stmt.go": {"NewAssignStmt"}, "types/type.go": {"NewArray"}, "typecheck/subr.go": {"NodAddr"}, "typecheck/typecheck.go": {"Call", "Stmt", "ConvNop"}, "typecheck/syms.go": {"LookupRuntime", "InitCoverage", "LookupCoverage"}},
	}
	units := []Unit{bloop, cover}
	for i, table := range calls {
		files := make([]string, 0, len(table))
		for file := range table {
			files = append(files, file)
		}
		sort.Strings(files)
		for _, file := range files {
			for _, name := range table[file] {
				prefix := map[string]string{"ir": "Typecheck.IR.", "types": "Typecheck.IR.Types.", "typecheck": "Typecheck."}[strings.SplitN(file, "/", 2)[0]]
				method := prefix + name
				if strings.HasPrefix(file, "base/") {
					method = strings.ToLower(name[:1]) + name[1:]
				}
				units[i].Calls = append(units[i].Calls, CallRule{Source: root + file, Function: name, Method: method})
			}
		}
	}
	return append([]Unit{
		{Source: "src/internal/coverage/defs.go", Functions: []string{"ParseCounterMode"}, Declarations: []string{"CounterMode", "CounterGranularity", "CtrModeInvalid", "CtrGranularityInvalid"}},
		{Source: "src/internal/coverage/pkid.go", WholeFile: true, ReadTables: []string{"rtPkgs"}},
	}, units...)
}

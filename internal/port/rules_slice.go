package port

func sliceMigration() []Unit {
	const root = "src/cmd/compile/internal/"
	return []Unit{{Source: root + "slice/slice.go", WholeFile: true,
		Context: &ContextRule{Type: "Context", Constructor: "newContext", Selective: true, PreserveNilArgs: true, Adapter: "adapter_context.go"},
		Checks:  map[string]string{"Funcs": "checkFuncs"},
		State: []StateRule{
			{Source: root + "base/flag.go", Variable: "Flag", Field: "Escape.Typecheck.IR.Base.Flag"},
			{Source: root + "base/debug.go", Variable: "Debug", Field: "Escape.Typecheck.IR.Debug"},
			{Source: root + "base/hashdebug.go", Variable: "VariableMakeHash", Field: "Escape.Typecheck.IR.Base.VariableMakeHash"},
		},
		Calls: []CallRule{
			{Source: root + "ir/dump.go", Function: "MatchAstDump", Method: "Escape.Typecheck.IR.MatchAstDump"},
			{Source: root + "ir/dump.go", Function: "AstDump", Method: "Escape.Typecheck.IR.AstDump"},
			{Source: root + "ir/expr.go", Function: "NewMoveToHeapExpr", Method: "Escape.Typecheck.IR.NewMoveToHeapExpr"},
			{Source: root + "ir/expr.go", Function: "StaticValue", Method: "Escape.Typecheck.IR.StaticValue"},
			{Source: root + "ir/expr.go", Function: "StaticCalleeName", Method: "Escape.Typecheck.IR.StaticCalleeName"},
			{Source: root + "ir/stmt.go", Function: "NewAssignStmt", Method: "Escape.Typecheck.IR.NewAssignStmt"},
			{Source: root + "escape/leaks.go", Function: "ParseLeaks", Method: "Escape.ParseLeaks"},
			{Source: root + "reflectdata/helpers.go", Function: "AppendElemRType", Method: "ReflectData.AppendElemRType"},
		},
	}}
}

package port

// The profile format and call graph are native inliner prerequisites. Raw pprof
// ingestion and the compiler's ambient file-opening entry point are not selected.
func pgoMigration() []Unit {
	return []Unit{
		{Source: "src/cmd/internal/pgo/pgo.go", WholeFile: true},
		{Source: "src/cmd/internal/pgo/serialize.go", WholeFile: true},
		{Source: "src/cmd/internal/pgo/deserialize.go", WholeFile: true},
		{Source: "src/cmd/compile/internal/pgoir/irgraph.go",
			Declarations: []string{"IRGraph", "IRNode", "IRNode.Name", "IREdge", "CallSiteInfo", "Profile", "Profile.PrintWeightedCallGraphDOT"},
			Functions:    []string{"createIRGraph", "visitIR", "createIRGraphEdge", "NodeLineOffset", "addIREdge", "addIndirectEdges", "DirectCallee"},
			Context:      &ContextRule{Type: "Context", Constructor: "newContext", Selective: true, PreserveNilArgs: true, RedirectTrace: true, Adapter: "adapter_context.go", Fields: map[string]string{"LookupFunc": "LookupFunc", "PostLookupCleanup": "PostLookupCleanup"}},
			Owner:        &OwnerRule{Types: []string{"Profile"}, Context: "Context", Field: "compilation"},
			State: []StateRule{
				{Source: "src/cmd/compile/internal/base/link.go", Variable: "Ctxt", Field: "Typecheck.IR.Base.Ctxt"},
				{Source: "src/cmd/compile/internal/base/debug.go", Variable: "Debug", Field: "Typecheck.IR.Debug"},
				{Source: "src/cmd/compile/internal/typecheck/target.go", Variable: "Target", Field: "Typecheck.Target"},
			},
			Calls: []CallRule{
				{Source: "src/cmd/compile/internal/ir/scc.go", Function: "VisitFuncsBottomUp", Method: "Typecheck.IR.VisitFuncsBottomUp"},
				{Source: "src/cmd/compile/internal/ir/expr.go", Function: "MethodExprName", Method: "Typecheck.IR.MethodExprName"},
				{Source: "src/cmd/compile/internal/ir/expr.go", Function: "StaticValue", Method: "Typecheck.IR.StaticValue"},
				{Source: "src/cmd/compile/internal/base/print.go", Function: "Fatalf", Method: "fatalf"},
				{Source: "src/cmd/compile/internal/types/identity.go", Function: "Identical", Method: "Typecheck.IR.Types.Identical"},
			},
			Checks: map[string]string{"NodeLineOffset": "checkOffset", "DirectCallee": "checkNode"},
		},
	}
}

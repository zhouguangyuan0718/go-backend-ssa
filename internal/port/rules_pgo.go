package port

// The complete native profile ingestion path uses owned decoder tables and
// error identities, with file opening supplied by the compiler instance.
func pgoMigration() []Unit {
	return []Unit{
		{Source: "src/internal/profile/profile.go", Functions: []string{"Parse", "parseUncompressed"}, Declarations: []string{"Profile", "ValueType", "Sample", "Label", "Mapping", "Location", "Line", "Function", "Profile.CheckValid", "Profile.Write"}, Context: &ContextRule{Type: "Context", Constructor: "newContext", Selective: true, Adapter: "adapter_context.go", Fields: map[string]string{"errMalformed": "errMalformed", "ErrNoData": "ErrNoData"}}, Owner: &OwnerRule{Types: []string{"Profile", "ValueType", "Sample", "Label", "Mapping", "Location", "Line", "Function"}, Context: "Context", Field: "compilation"}},
		{Source: "src/internal/profile/encode.go", WholeFile: true, Context: &ContextRule{Type: "Context", Constructor: "newContext", Selective: true, Adapter: "adapter_context.go", Fields: map[string]string{"profileDecoder": "profileDecoder", "valueTypeDecoder": "valueTypeDecoder", "sampleDecoder": "sampleDecoder", "labelDecoder": "labelDecoder", "mappingDecoder": "mappingDecoder", "locationDecoder": "locationDecoder", "lineDecoder": "lineDecoder", "functionDecoder": "functionDecoder"}}},
		{Source: "src/internal/profile/proto.go", WholeFile: true},
		{Source: "src/internal/profile/graph.go", WholeFile: true},
		{Source: "src/cmd/internal/pgo/pprof.go", WholeFile: true, Context: &ContextRule{Type: "Context", Constructor: "newContext", Selective: true, Adapter: "adapter_context.go"}, State: []StateRule{{Source: "src/internal/profile/profile.go", Variable: "ErrNoData", Field: "Profiles.ErrNoData"}}, Calls: []CallRule{{Source: "src/internal/profile/profile.go", Function: "Parse", Method: "Profiles.Parse"}}},
		{Source: "src/cmd/internal/pgo/pgo.go", WholeFile: true},
		{Source: "src/cmd/internal/pgo/serialize.go", WholeFile: true},
		{Source: "src/cmd/internal/pgo/deserialize.go", WholeFile: true},
		{Source: "src/cmd/compile/internal/pgoir/irgraph.go",
			Declarations:   []string{"IRGraph", "IRNode", "IRNode.Name", "IREdge", "CallSiteInfo", "Profile", "Profile.PrintWeightedCallGraphDOT"},
			ContextMethods: map[string]string{"New": "readProfile"},
			HostCalls:      []HostCallRule{{Package: "os", Function: "Open", Method: "openProfile", StreamMethods: []string{"Read", "Close"}}},
			Functions:      []string{"New", "createIRGraph", "visitIR", "createIRGraphEdge", "NodeLineOffset", "addIREdge", "addIndirectEdges", "DirectCallee"},
			Context:        &ContextRule{Type: "Context", Constructor: "newContext", Selective: true, PreserveNilArgs: true, RedirectTrace: true, Adapter: "adapter_context.go", Fields: map[string]string{"LookupFunc": "LookupFunc", "PostLookupCleanup": "PostLookupCleanup"}},
			Owner:          &OwnerRule{Types: []string{"Profile"}, Context: "Context", Field: "compilation"},
			State: []StateRule{
				{Source: "src/cmd/compile/internal/base/link.go", Variable: "Ctxt", Field: "Typecheck.IR.Base.Ctxt"},
				{Source: "src/cmd/compile/internal/base/debug.go", Variable: "Debug", Field: "Typecheck.IR.Debug"},
				{Source: "src/cmd/compile/internal/typecheck/target.go", Variable: "Target", Field: "Typecheck.Target"},
			},
			Calls: []CallRule{
				{Source: "src/cmd/internal/pgo/pprof.go", Function: "FromPProf", Method: "processing.FromPProf"},
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

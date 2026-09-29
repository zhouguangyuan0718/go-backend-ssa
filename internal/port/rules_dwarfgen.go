package port

func dwarfgenMigration() []Unit {
	context := func() *ContextRule {
		return &ContextRule{Type: "Context", Constructor: "newContext", Adapter: "adapter_context.go", Selective: true}
	}
	return []Unit{
		{Source: "src/cmd/compile/internal/dwarfgen/marker.go", WholeFile: true, Context: context(), Owner: &OwnerRule{Types: []string{"ScopeMarker"}, Context: "Context", Field: "compilation"}, Calls: []CallRule{
			{Source: "src/cmd/compile/internal/base/print.go", Function: "Fatalf", Method: "fatalf"},
			{Source: "src/cmd/compile/internal/base/print.go", Function: "FatalfAt", Method: "fatalfAt"},
			{Source: "src/cmd/compile/internal/base/print.go", Function: "FmtPos", Method: "Base.FmtPos"},
		}},
		{Source: "src/cmd/compile/internal/dwarfgen/scope.go", Functions: []string{"xposBefore"}, Context: context(), State: []StateRule{{Source: "src/cmd/compile/internal/base/link.go", Variable: "Ctxt", Field: "Base.Ctxt"}}},
	}
}

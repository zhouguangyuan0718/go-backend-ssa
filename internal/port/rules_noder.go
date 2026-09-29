package port

func noderPositions() []Unit {
	context := func() *ContextRule {
		return &ContextRule{Type: "Context", Constructor: "newContext", Adapter: "adapter_context.go", Selective: true}
	}
	return []Unit{
		{Source: "src/cmd/compile/internal/noder/noder.go", Functions: []string{"trimFilename"}, Context: context(),
			State: []StateRule{{Source: "src/cmd/compile/internal/base/link.go", Variable: "Ctxt", Field: "Ctxt"}, {Source: "src/cmd/compile/internal/base/flag.go", Variable: "Flag", Field: "Flag"}},
			Calls: []CallRule{{Source: "src/cmd/internal/objabi/line.go", Function: "AbsFile", Method: "absFile"}}},
		{Source: "src/cmd/compile/internal/noder/posmap.go", WholeFile: true, Context: context(), Owner: &OwnerRule{Types: []string{"posMap"}, Context: "Context", Field: "compilation"}},
	}
}

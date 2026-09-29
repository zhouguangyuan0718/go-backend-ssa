package port

// Keep the full native range-over-function transformation. Predeclared objects
// and the synthetic runtime helper package belong to the caller's checker.
func rangefuncMigration() []Unit {
	return []Unit{
		{Source: "src/internal/abi/rangefuncconsts.go", WholeFile: true},
		{Source: "src/cmd/compile/internal/rangefunc/rewrite.go", WholeFile: true, ZeroValues: []string{"nopos"},
			Context: &ContextRule{Type: "Context", Constructor: "initialize", Initialize: true, Selective: true, PreserveNilArgs: true, RedirectTrace: true, Adapter: "adapter_context.go", Fields: map[string]string{"runtimePkg": "runtimePkg"}},
			Owner:   &OwnerRule{Types: []string{"rewriter"}, Context: "Context", Field: "compilation"},
			State: []StateRule{
				{Source: "src/cmd/compile/internal/base/flag.go", Variable: "Flag", Field: "Flag"},
				{Source: "src/cmd/compile/internal/base/debug.go", Variable: "Debug", Field: "Debug"},
				{Source: "src/cmd/compile/internal/types2/universe.go", Variable: "Universe", Field: "Checker.Universe"},
			},
			Calls: []CallRule{
				{Source: "src/cmd/compile/internal/base/print.go", Function: "Fatalf", Method: "fatalf"},
				{Source: "src/cmd/compile/internal/types2/compilersupport.go", Function: "CoreType", Method: "Checker.CoreType"},
				{Source: "src/cmd/compile/internal/types2/object.go", Function: "NewVar", Method: "Checker.NewVar"},
				{Source: "src/cmd/compile/internal/types2/object.go", Function: "NewParam", Method: "Checker.NewParam"},
				{Source: "src/cmd/compile/internal/types2/object.go", Function: "NewFunc", Method: "Checker.NewFunc"},
				{Source: "src/cmd/compile/internal/types2/signature.go", Function: "NewSignatureType", Method: "Checker.NewSignatureType"},
				{Source: "src/cmd/compile/internal/types2/tuple.go", Function: "NewTuple", Method: "Checker.NewTuple"},
				{Source: "src/cmd/compile/internal/types2/package.go", Function: "NewPackage", Method: "Checker.NewPackage"},
			},
		},
	}
}

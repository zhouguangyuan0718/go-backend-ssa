package port

// Baseline pins the official https://go.googlesource.com/go source.
// Return fresh values so upgrade tests can change the pin without shared state.
func Baseline() Pin {
	return Pin{
		Tag:            "go1.27.1",
		Commit:         "862c888e612ac346c7c4d99c9392bdfd265f33b0",
		BuildToolchain: "go1.27.1",
		TargetModule:   "go-backend-ssa",
	}
}

// Migration defines the selected source and the adaptations maintained here.
// Paths/imports are derived; package names come from the upstream AST.
// InspectRoots cover candidate dependencies beyond the currently ported files.
func Migration() Plan {
	return Plan{
		InspectRoots: []string{
			"cmd/compile/internal/gc",
			"cmd/compile/internal/syntax",
			"cmd/compile/internal/types2",
			"cmd/compile/internal/noder",
			"cmd/compile/internal/ir",
			"cmd/compile/internal/types",
			"cmd/compile/internal/typecheck",
			"cmd/compile/internal/ssagen",
			"cmd/compile/internal/ssa",
		},
		Units: []Unit{
			{
				Source:    "src/cmd/internal/objabi/path.go",
				Functions: []string{"PathToPrefix", "PrefixToPath"},
			},
			{
				Source:    "src/cmd/compile/internal/ir/func.go",
				Functions: []string{"ParseLinkFuncName", "splitPkg", "escapedImportPathOK", "modPathOK"},
			},
			{
				Source:    "src/cmd/compile/internal/types/pkg.go",
				Functions: []string{"InternString"},
				Context: &ContextRule{
					Type: "Context", Constructor: "newContext",
					Adapter: "adapter_context.go",
					Fields: map[string]string{
						"internedStringsmu": "mu",
						"internedStrings":   "strings",
					},
				},
			},
		},
	}
}

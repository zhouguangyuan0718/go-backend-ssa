package port

func walkMigration() []Unit {
	const root = "src/cmd/compile/internal/"
	context := func() *ContextRule {
		return &ContextRule{Type: "Context", Constructor: "newContext", Adapter: "adapter_context.go", Selective: true}
	}
	return []Unit{
		{Source: root + "walk/walk.go", Functions: []string{"mayCall"}, Context: context(),
			State: []StateRule{
				{Source: root + "ssagen/arch.go", Variable: "Arch.SoftFloat", Field: "SoftFloat"},
				{Source: root + "base/flag.go", Variable: "Flag", Field: "Typecheck.IR.Base.Flag"},
				{Source: root + "types/type.go", Variable: "IsFloat", Field: "Typecheck.IR.Types.IsFloat"},
				{Source: root + "types/type.go", Variable: "IsComplex", Field: "Typecheck.IR.Types.IsComplex"},
			},
			Calls: []CallRule{{Source: root + "base/print.go", Function: "FatalfAt", Method: "fatalfAt"}},
		},
		{Source: root + "walk/range.go", Functions: []string{"cheapComputableIndex"}, Context: context(), State: []StateRule{
			{Source: root + "ssagen/arch.go", Variable: "Arch.LinkArch", Field: "Typecheck.IR.Base.Ctxt.Arch"},
		}},
	}
}

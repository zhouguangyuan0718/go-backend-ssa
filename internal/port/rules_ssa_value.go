package port

// Keep the native predicate signature: its target state is already owned by t.
func ssaValueMigration() []Unit {
	const root = "src/cmd/compile/internal/"
	return []Unit{
		{Source: root + "ssa/decompose.go", Declarations: []string{"MaxStruct"}},
		{Source: root + "ssa/value.go", Functions: []string{"CanSSA"},
			Context:     &ContextRule{Type: "typeContext", Constructor: "newTypeContext", Adapter: "adapter_context.go", Selective: true},
			ContextFrom: map[string]ContextSource{"CanSSA": {Argument: "t", Resolver: "typeContextOf"}},
			State: []StateRule{
				{Source: root + "types/size.go", Variable: "PtrSize", Field: "ptrSize"},
				{Source: "src/internal/buildcfg/exp.go", Variable: "Experiment", Field: "Types.Experiment"},
			},
			Calls: []CallRule{
				{Source: root + "types/size.go", Function: "CalcSize", Method: "Types.CalcSize"},
				{Source: root + "types/type.go", Function: "IsDirectIface", Method: "Types.IsDirectIface"},
			},
		},
	}
}

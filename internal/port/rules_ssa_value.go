package port

// Keep the native predicate signature: its target state is already owned by t.
func ssaValueMigration() []Unit {
	const root = "src/cmd/compile/internal/"
	return []Unit{
		{Source: root + "ssa/decompose.go", Declarations: []string{"MaxStruct"}},
		{Source: root + "ssa/value.go", Functions: []string{"CanSSA"}, Declarations: []string{"Value", "Value.String", "Value.AddArg", "Value.AddArg2", "Value.AddArg3", "Value.AddArg4", "Value.AddArg5", "Value.AddArg6", "Value.AddArgs", "Value.SetArg", "Value.SetArgs1", "Value.SetArgs2", "Value.SetArgs3", "Value.SetArgs4", "Value.resetArgs", "Value.isGenericIntConst", "Value.Logf", "Value.Log"},
			Context:     &ContextRule{Type: "Context", Constructor: "initialize", Initialize: true, Adapter: "adapter_context.go", Selective: true},
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

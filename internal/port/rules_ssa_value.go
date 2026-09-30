package port

// Keep the native predicate signature: its target state is already owned by t.
func ssaValueMigration() []Unit {
	const root = "src/cmd/compile/internal/"
	return []Unit{
		{Source: root + "ssa/decompose.go", Declarations: []string{"MaxStruct"}},
		{Source: root + "ssa/value.go", WholeFile: true,
			Context:     &ContextRule{Type: "Context", Constructor: "initialize", Initialize: true, Adapter: "adapter_context.go", Selective: true},
			ContextFrom: map[string]ContextSource{"CanSSA": {Argument: "t", Resolver: "typeContextOf"}, "Value.LackingPos": {Argument: "v", Resolver: "valueContext"}},
			State: []StateRule{
				{Source: root + "types/type.go", Variable: "TypeMem", Field: "Types.TypeMem"},
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

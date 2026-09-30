package port

// These are the native Config inputs. Keep their declarations independent of
// Config's architecture rewrite closure; do not invent substitute graph types.
func ssaTypesMigration() []Unit {
	const root = "src/cmd/compile/internal/"
	u := Unit{Source: root + "ssa/config.go",
		Declarations: []string{"Types", "Types.SetTypPtrs", "Logger", "Frontend"},
		Functions:    []string{"NewTypes"},
		Context:      &ContextRule{Type: "typeContext", Constructor: "newTypeContext", Adapter: "adapter_context.go", Selective: true},
		Owner:        &OwnerRule{Types: []string{"Types"}, Context: "typeContext", Field: "compilation"},
		Checks:       map[string]string{"NewTypes": "checkTypes", "Types.SetTypPtrs": "checkTypes"},
		Calls:        []CallRule{{Source: root + "types/type.go", Function: "NewPtr", Method: "Types.NewPtr"}},
	}
	for _, name := range []string{"Types", "TypeVec128", "TypeVec256", "TypeVec512", "TypeMask"} {
		u.State = append(u.State, StateRule{Source: root + "types/type.go", Variable: name, Field: "Types." + name})
	}
	return []Unit{u,
		{Source: root + "ssa/location.go", WholeFile: true},
		{Source: root + "ssa/id.go", WholeFile: true},
	}
}

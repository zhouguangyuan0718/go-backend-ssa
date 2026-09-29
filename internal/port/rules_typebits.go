package port

func bitvecMigration() Unit {
	return Unit{Source: "src/cmd/compile/internal/bitvec/bv.go", WholeFile: true,
		Context: &ContextRule{Type: "fatalContext", Constructor: "newFatalContext", Adapter: "adapter_fatal.go", Selective: true},
		ContextFrom: map[string]ContextSource{
			"BitVec.Eq":    {Argument: "bv1", Resolver: "bitvecFatal"},
			"BitVec.Get":   {Argument: "bv", Resolver: "bitvecFatal"},
			"BitVec.Set":   {Argument: "bv", Resolver: "bitvecFatal"},
			"BitVec.Unset": {Argument: "bv", Resolver: "bitvecFatal"},
		},
		Calls: []CallRule{{Source: "src/cmd/compile/internal/base/print.go", Function: "Fatalf", Method: "fatalf"}},
	}
}
func typebitsMigration() []Unit {
	return []Unit{{Source: "src/cmd/compile/internal/typebits/typebits.go", WholeFile: true,
		Context: &ContextRule{Type: "typeContext", Constructor: "newTypeContext", Adapter: "adapter_context.go", Selective: true},
		ContextFrom: map[string]ContextSource{
			"Set":        {Argument: "t", Resolver: "typeContextOf"},
			"SetNoCheck": {Argument: "t", Resolver: "typeContextOf"},
			"set":        {Argument: "t", Resolver: "typeContextOf"},
		},
		State: []StateRule{{Source: "src/cmd/compile/internal/types/size.go", Variable: "PtrSize", Field: "ptrSize"}},
		Calls: []CallRule{{Source: "src/cmd/compile/internal/base/print.go", Function: "Fatalf", Method: "fatalf"}},
	}}
}

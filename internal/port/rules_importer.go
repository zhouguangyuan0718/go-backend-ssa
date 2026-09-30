package port

// The compiler's type-checker importer retains its lazy declarations, recursive
// dictionaries and delayed constraints. File discovery remains a host boundary.
func importerMigration() []Unit {
	context := func() *ContextRule {
		return &ContextRule{Type: "ImportContext", Constructor: "newContext", Adapter: "adapter_context.go", Selective: true, PreserveNilArgs: true}
	}
	u := Unit{Source: "src/cmd/compile/internal/importer/ureader.go", WholeFile: true, Context: context(), Owner: &OwnerRule{Types: []string{"pkgReader", "reader"}, Context: "ImportContext", Field: "compilation"},
		Checks:      map[string]string{"ReadPackage": "checkRead"},
		State:       []StateRule{{Source: "src/cmd/compile/internal/types2/universe.go", Variable: "Typ", Field: "checker.Typ"}, {Source: "src/cmd/compile/internal/types2/universe.go", Variable: "Unsafe", Field: "checker.Unsafe"}},
		MethodCalls: []MethodCallRule{{Source: "src/cmd/compile/internal/types2/package.go", Method: "Package.Scope", Adapter: "checker.PackageScope"}},
		Calls:       []CallRule{{Source: "src/cmd/compile/internal/base/print.go", Function: "Fatalf", Method: "fatalf"}, {Source: "src/cmd/compile/internal/base/print.go", Function: "FatalfAt", Method: "fatalfAt"}, {Source: "src/cmd/compile/internal/base/print.go", Function: "Assertf", Method: "assertf"}},
	}
	for _, pair := range [][2]string{
		{"package.go", "NewPackage"}, {"instantiate.go", "Instantiate"}, {"array.go", "NewArray"}, {"chan.go", "NewChan"}, {"map.go", "NewMap"}, {"pointer.go", "NewPointer"}, {"slice.go", "NewSlice"}, {"object.go", "NewField"}, {"struct.go", "NewStruct"}, {"union.go", "NewUnion"}, {"object.go", "NewFunc"}, {"interface.go", "NewInterfaceType"}, {"signature.go", "NewSignatureType"}, {"tuple.go", "NewTuple"}, {"object.go", "NewParam"}, {"object.go", "NewConst"}, {"object.go", "NewTypeNameLazy"}, {"object.go", "NewVar"}, {"object.go", "NewTypeName"}, {"typeparam.go", "NewTypeParam"}, {"alias.go", "NewAlias"},
	} {
		u.Calls = append(u.Calls, CallRule{Source: "src/cmd/compile/internal/types2/" + pair[0], Function: pair[1], Method: "checker." + pair[1]})
	}
	return []Unit{u, {Source: "src/cmd/compile/internal/importer/support.go", Functions: []string{"assert"}, Declarations: []string{"derivedInfo", "typeInfo"}, Context: context(), Calls: []CallRule{{Source: "src/cmd/compile/internal/base/print.go", Function: "Assert", Method: "assertCondition"}}}}
}

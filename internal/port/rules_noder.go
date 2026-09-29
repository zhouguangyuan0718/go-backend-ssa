package port

func noderMigration() []Unit {
	context := func() *ContextRule {
		return &ContextRule{Type: "Context", Constructor: "initContext", Initialize: true, PreserveNilArgs: true, RedirectTrace: true, Adapter: "adapter_context.go", Selective: true}
	}
	return []Unit{
		{Source: "src/cmd/compile/internal/noder/noder.go", Functions: []string{"trimFilename", "checkEmbed"}, Declarations: []string{"noder", "linkname", "pragmas", "pragmas.Nointerface", "WasmImport", "WasmExport", "pragmaPos", "pragmaEmbed"},
			Context: func() *ContextRule {
				c := context()
				c.Fields = map[string]string{"unOps": "unOps", "binOps": "binOps"}
				return c
			}(),
			State: []StateRule{{Source: "src/cmd/compile/internal/base/link.go", Variable: "Ctxt", Field: "Ctxt"}, {Source: "src/cmd/compile/internal/base/flag.go", Variable: "Flag", Field: "Flag"}},
			Calls: []CallRule{{Source: "src/cmd/internal/objabi/line.go", Function: "AbsFile", Method: "absFile"}}},
		{Source: "src/cmd/compile/internal/noder/posmap.go", WholeFile: true, Context: context(), Owner: &OwnerRule{Types: []string{"posMap"}, Context: "Context", Field: "compilation"}},
		{Source: "src/cmd/compile/internal/noder/codes.go", WholeFile: true},
		{Source: "src/cmd/compile/internal/noder/lex.go", Declarations: []string{"funcPragmas"}},
		{Source: "src/cmd/compile/internal/noder/quirks.go", Functions: []string{"typeExprEndPos", "lastFieldType"}, Context: context()},
		{Source: "src/cmd/compile/internal/noder/helpers.go", Functions: []string{"idealType", "isTypeParam", "isNotInHeap"}, Context: context()},
		{Source: "src/cmd/compile/internal/noder/types.go", Functions: []string{"deref2"}, Context: context()},
		{Source: "src/cmd/compile/internal/noder/unified.go", Declarations: []string{"uirVersion"}, Functions: []string{"writePkgStub"}, Context: context(), Calls: []CallRule{
			{Source: "src/cmd/compile/internal/noder/irgen.go", Function: "checkFiles", Method: "checkedFiles"},
			{Source: "src/cmd/compile/internal/noder/unified.go", Function: "freePackage", Method: "retainCheckedPackage"},
		}},
		{Source: "src/cmd/compile/internal/noder/writer.go", WholeFile: true,
			Context: func() *ContextRule {
				c := context()
				c.Fields = map[string]string{"anyTypeName": "anyTypeName", "comparableTypeName": "comparableTypeName", "runeTypeName": "runeTypeName"}
				return c
			}(),
			Owner:         &OwnerRule{Types: []string{"pkgWriter", "writer", "writerDict", "declCollector"}, Context: "Context", Field: "compilation"},
			ContextParams: []string{"objInfo.*"},
			State: []StateRule{
				{Source: "src/cmd/compile/internal/base/debug.go", Variable: "Debug", Field: "Debug"},
				{Source: "src/internal/buildcfg/cfg.go", Variable: "GOARCH", Field: "GOARCH"},
				{Source: "src/cmd/compile/internal/types2/universe.go", Variable: "Universe", Field: "Checker.Universe"},
				{Source: "src/cmd/compile/internal/types2/universe.go", Variable: "Unsafe", Field: "Checker.Unsafe"},
				{Source: "src/cmd/compile/internal/types2/universe.go", Variable: "Typ", Field: "Checker.Typ"},
			},
			Calls: []CallRule{
				{Source: "src/cmd/compile/internal/noder/writer.go", Function: "isGlobal", Method: "isGlobal"},
				{Source: "src/cmd/compile/internal/base/print.go", Function: "Assert", Method: "assertCondition"},
				{Source: "src/cmd/compile/internal/base/print.go", Function: "Assertf", Method: "assertf"},
				{Source: "src/cmd/compile/internal/base/print.go", Function: "Fatalf", Method: "fatalf"},
				{Source: "src/cmd/compile/internal/base/print.go", Function: "FatalfAt", Method: "fatalfAt"},
				{Source: "src/cmd/compile/internal/base/print.go", Function: "ErrorfAt", Method: "errorfAt"},
				{Source: "src/cmd/compile/internal/types/goversion.go", Function: "AllowsGoVersion", Method: "allowsGoVersion"},
				{Source: "src/cmd/compile/internal/types/fmt.go", Function: "SplitVargenSuffix", Method: "Types.SplitVargenSuffix"},
				{Source: "src/cmd/compile/internal/types2/api_predicates.go", Function: "AssignableTo", Method: "Checker.AssignableTo"},
				{Source: "src/cmd/compile/internal/types2/api_predicates.go", Function: "Identical", Method: "Checker.Identical"},
				{Source: "src/cmd/compile/internal/types2/compilersupport.go", Function: "CoreType", Method: "Checker.CoreType"},
				{Source: "src/cmd/compile/internal/types2/compilersupport.go", Function: "RangeKeyVal", Method: "Checker.RangeKeyVal"},
				{Source: "src/cmd/compile/internal/types2/predicates.go", Function: "Default", Method: "Checker.Default"},
				{Source: "src/cmd/compile/internal/types2/interface.go", Function: "NewInterfaceType", Method: "Checker.NewInterfaceType"},
				{Source: "src/cmd/compile/internal/types2/lookup.go", Function: "LookupFieldOrMethod", Method: "Checker.LookupFieldOrMethod"},
			},
		},
	}
}

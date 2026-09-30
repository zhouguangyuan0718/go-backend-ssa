package port

// Keep the entire native checkFiles and unified bodies. Host adapters only bind
// each fresh checker and turn process exits into instance diagnostics.
func noderSourceMigration(units []Unit) []Unit {
	const root = "src/cmd/compile/internal/"
	for i := range units {
		u := &units[i]
		switch u.Source {
		case root + "noder/writer.go":
			u.Context.InitGroup = "initializeWriterUniverse"
		case root + "noder/unified.go":
			u.Functions = append(u.Functions, "unified")
			for j := range u.Calls {
				if u.Calls[j].Function == "checkFiles" {
					u.Calls[j].KeepDefinition = true
				}
			}
			u.State = append(u.State,
				StateRule{Source: root + "inline/inl.go", Variable: "InlineCall", Field: "Inline.InlineCall"},
				StateRule{Source: root + "typecheck/iimport.go", Variable: "HaveInlineBody", Field: "Typecheck.HaveInlineBody"},
				StateRule{Source: root + "pgoir/irgraph.go", Variable: "LookupFunc", Field: "Inline.PGO.LookupFunc"},
				StateRule{Source: root + "pgoir/irgraph.go", Variable: "PostLookupCleanup", Field: "Inline.PGO.PostLookupCleanup"},
			)
			u.Calls = append(u.Calls, CallRule{Source: root + "types/type.go", Function: "RuntimeSymName", Method: "Types.RuntimeSymName"})
		}
	}
	return append(units, Unit{Source: root + "noder/irgen.go", WholeFile: true,
		Context: &ContextRule{Type: "Context", Constructor: "initContext", Initialize: true, Selective: true, PreserveNilArgs: true, RedirectTrace: true, Adapter: "adapter_context.go", Fields: map[string]string{"versionErrorRx": "versionErrorRx"}},
		Owner:   &OwnerRule{Types: []string{"cycleFinder"}, Context: "Context", Field: "compilation"},
		Calls: []CallRule{
			{Source: root + "types2/context.go", Function: "NewContext", Method: "freshChecker"},
			{Source: root + "types2/sizes.go", Function: "SizesFor", Method: "Checker.SizesFor"},
			{Source: root + "base/print.go", Function: "SyntaxErrors", Method: "syntaxErrors"},
			{Source: root + "base/print.go", Function: "ExitIfErrors", Method: "exitIfErrors"},
			{Source: root + "midway/rewrite.go", Function: "RewriteWrapper", Method: "rewriteMidway"},
			{Source: root + "rangefunc/rewrite.go", Function: "Rewrite", Method: "rewriteRangeFuncs"},
		},
	})
}

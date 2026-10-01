package port

func linkMigration() []Unit {
	return []Unit{
		{Source: "src/cmd/compile/internal/base/hashdebug.go", WholeFile: true,
			AdapterMethods: []string{"HashDebug.log"},
			Owner:          &OwnerRule{Types: []string{"HashDebug"}, Context: "LinkContext", Field: "compilation"},
			Nullable:       map[string][]string{"HashDebug.MatchPkgFunc": {"d"}, "HashDebug.MatchPos": {"d"}, "HashDebug.MatchPosWithInfo": {"d"}},
			Context: &ContextRule{Type: "LinkContext", Constructor: "newLinkContext", Adapter: "adapter_link.go", Selective: true, Fields: map[string]string{
				"hashDebug": "hashDebug", "ConvertHash": "ConvertHash", "FmaHash": "FmaHash", "LoopVarHash": "LoopVarHash", "PGOHash": "PGOHash", "LiteralAllocHash": "LiteralAllocHash", "MergeLocalsHash": "MergeLocalsHash", "VariableMakeHash": "VariableMakeHash",
			}},
			Calls: []CallRule{{Source: "src/cmd/compile/internal/base/print.go", Function: "Fatalf", Method: "debugFlagFatalf"}},
		},
		{Source: "src/cmd/compile/internal/base/timings.go", WholeFile: true, Context: &ContextRule{Type: "LinkContext", Constructor: "newLinkContext", Adapter: "adapter_link.go", Selective: true, Fields: map[string]string{"Timer": "Timer"}}},
		{Source: "src/cmd/internal/obj/abi_string.go", WholeFile: true, ReadTables: []string{"_ABI_index"}},
		{Source: "src/cmd/internal/obj/sym.go", Declarations: []string{"StaticNamePrefix", "Link.LookupABI", "Link.LookupABIInit", "Link.Lookup", "Link.LookupInit", "Link.LookupDerived", "Link.LookupStatic"}},
		{Source: "src/cmd/compile/internal/base/link.go", Functions: []string{"PkgLinksym", "Linkname", "linksym"}, Context: &ContextRule{Type: "LinkContext", Constructor: "newLinkContext", Adapter: "adapter_link.go", Selective: true, Fields: map[string]string{"ReservedImports": "ReservedImports", "Ctxt": "Ctxt"}}},
		{Source: "src/cmd/compile/internal/base/print.go", Functions: []string{"FmtPos", "addErrorMsg", "FlushErrors", "WarnfAt"}, Declarations: []string{"errorMsg", "byPos", "byPos.Len", "byPos.Less", "byPos.Swap"}, ContextMethods: map[string]string{"WarnfAt": "reportWarning", "FlushErrors": "flushReports"}, Context: &ContextRule{Type: "LinkContext", Constructor: "newLinkContext", Adapter: "adapter_link.go", Selective: true, Fields: map[string]string{"errorMsgs": "reportMessages"}}},
	}
}

package port

func linkMigration() []Unit {
	return []Unit{
		{Source: "src/cmd/compile/internal/base/timings.go", WholeFile: true, Context: &ContextRule{Type: "LinkContext", Constructor: "newLinkContext", Adapter: "adapter_link.go", Selective: true, Fields: map[string]string{"Timer": "Timer"}}},
		{Source: "src/cmd/internal/obj/abi_string.go", WholeFile: true, ReadTables: []string{"_ABI_index"}},
		{Source: "src/cmd/internal/obj/sym.go", Declarations: []string{"Link.LookupABI", "Link.LookupABIInit", "Link.Lookup", "Link.LookupInit"}},
		{Source: "src/cmd/compile/internal/base/link.go", Functions: []string{"PkgLinksym", "Linkname", "linksym"}, Context: &ContextRule{Type: "LinkContext", Constructor: "newLinkContext", Adapter: "adapter_link.go", Selective: true, Fields: map[string]string{"ReservedImports": "ReservedImports", "Ctxt": "Ctxt"}}},
		{Source: "src/cmd/compile/internal/base/print.go", Functions: []string{"FmtPos"}, Context: &ContextRule{Type: "LinkContext", Constructor: "newLinkContext", Adapter: "adapter_link.go", Selective: true}},
	}
}

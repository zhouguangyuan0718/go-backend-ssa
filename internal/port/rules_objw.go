package port

func objectDataContext() *ContextRule {
	return &ContextRule{Existing: true, Initialize: true, Selective: true, PreserveNilArgs: true, Type: "Link", Constructor: "initializeData", Adapter: "adapter_data.go", Embed: "dataState", RedirectTrace: true, RedirectExit: true}
}
func objectDataMigration() []Unit {
	fips := objectDataContext()
	fips.Fields = map[string]string{"bisectFIPS": "bisectFIPS"}
	return []Unit{
		{Source: "src/internal/bisect/bisect.go", Functions: []string{"New", "Hash", "Marker", "AppendMarker", "fnv", "fnvString", "fnvUint64", "fnvUint32"}, Declarations: []string{"Matcher", "cond", "parseError", "parseError.Error", "Matcher.MarkerOnly", "Matcher.ShouldEnable", "Matcher.ShouldPrint", "Matcher.matchResult", "dedup", "offset64"}},
		{Source: "src/cmd/internal/obj/data.go", WholeFile: true, Context: objectDataContext(), ContextFrom: map[string]ContextSource{"LSym.Grow": {Argument: "s", Resolver: "statelessDataLink"}}},
		{Source: "src/cmd/internal/obj/fips140.go", WholeFile: true, Context: fips, ContextFrom: map[string]ContextSource{"LSym.setFIPSType": {Argument: "ctxt", Resolver: "dataLink"}}, State: []StateRule{
			{Source: "src/internal/buildcfg/cfg.go", Variable: "GOOS", Field: "TargetOS"},
			{Source: "src/internal/buildcfg/cfg.go", Variable: "GOARCH", Field: "Arch.Name"},
		}},
		{Source: "src/cmd/internal/obj/textflag.go", WholeFile: true},
		{Source: "src/cmd/internal/objabi/symkind_string.go", WholeFile: true, ReadTables: []string{"_SymKind_index"}},
		{Source: "src/cmd/internal/objabi/reloctype_string.go", WholeFile: true, ReadTables: []string{"_RelocType_index"}},
	}
}
func objwMigration() []Unit {
	return []Unit{
		bitvecMigration(),
		{Source: "src/cmd/compile/internal/objw/objw.go", WholeFile: true, Context: &ContextRule{Type: "Context", Constructor: "newContext", Adapter: "adapter_context.go", Selective: true}, State: []StateRule{
			{Source: "src/cmd/compile/internal/base/link.go", Variable: "Ctxt", Field: "Link"},
			{Source: "src/cmd/compile/internal/types/size.go", Variable: "PtrSize", Field: "ptrSize"},
		}, Calls: []CallRule{
			{Source: "src/cmd/compile/internal/base/print.go", Function: "Fatalf", Method: "fatalf"},
			{Source: "src/cmd/compile/internal/types/size.go", Function: "RoundUp", Method: "Types.RoundUp"},
		}},
	}
}

package port

// Preserve native HTML construction, phase-mask parsing and source collection.
// Host resources are supplied explicitly by the owning compiler instance.
func ssaSourceDiagnosticMigration(units []Unit) []Unit {
	const root = "src/cmd/compile/internal/"
	for i := range units {
		u := &units[i]
		switch u.Source {
		case root + "ssa/html.go":
			u.Functions = append(u.Functions, "NewHTMLWriter", "newDotWriter", "passIdxByName")
			u.ContextFrom["NewHTMLWriter"] = ContextSource{Argument: "f", Resolver: "diagnosticContext"}
			u.HostCalls = []HostCallRule{
				{Package: "os", Function: "OpenFile", Method: "openHTML"},
				{Package: "os", Function: "Getwd", Method: "workingDirectory"},
				{Package: "os/exec", Function: "LookPath", Method: "lookPath", StringArgument: "dot"},
			}
		case root + "ssagen/ssa.go":
			u.Functions = append(u.Functions, "InitEnv", "DumpInline", "readFuncLines", "dumpSourcesColumn")
			for _, name := range []string{"ssaDump", "ssaDir", "ssaDumpStdout", "ssaDumpCFG", "ssaDumpInlined"} {
				u.Context.Fields[name] = name
			}
			u.Declarations = append(u.Declarations, "ssaDumpFile")
			u.Checks["DumpInline"] = "checkDumpInline"
			u.HostCalls = []HostCallRule{
				{Package: "os", Function: "Getenv", Method: "dumpEnvironment", StringArguments: []string{"GOSSAFUNC", "GOSSADIR"}},
				{Package: "os", Function: "Open", Method: "openSource"},
				{Package: "os", Function: "ExpandEnv", Method: "expandSourcePath"},
			}
		}
	}
	return units
}

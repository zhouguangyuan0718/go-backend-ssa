package port

// Retain native text and HTML IR reports, with file services and report tables
// owned by the same IR instance that owns the nodes being dumped.
func irDiagnosticMigration(units []Unit) []Unit {
	const root = "src/cmd/compile/internal/ir/"
	for i := range units {
		u := &units[i]
		switch u.Source {
		case root + "dump.go":
			u.Functions = nil
			u.Declarations = nil
			u.WholeFile = true
			u.Context.Fields = map[string]string{"mu": "astMu", "astDumpFiles": "astDumpFiles", "htmlWriters": "astHTMLWriters", "orderedFuncs": "astOrderedFuncs", "indentBytes": "dumpIndentBytes"}
			u.Owner = &OwnerRule{Types: []string{"dumper"}, Context: "Context", Field: "compilation"}
			u.HostCalls = []HostCallRule{{Package: "os", Function: "OpenFile", Method: "openAST"}}
			for _, binding := range [][2]string{{"O_APPEND", "astAppendFlag"}, {"O_RDWR", "astReadWriteFlag"}, {"O_CREATE", "astCreateFlag"}, {"O_TRUNC", "astTruncateFlag"}} {
				u.HostConstants = append(u.HostConstants, HostConstantRule{Package: "os", Constant: binding[0], Field: binding[1]})
			}
			u.Checks = map[string]string{"AstDump": "checkAstDump", "DumpNodeHTML": "checkDumpNodeHTML", "FDumpAny": "checkDumpAny"}
		case root + "html.go":
			u.Functions = nil
			u.Declarations = nil
			u.WholeFile = true
			u.Owner.Types = append(u.Owner.Types, "HTMLWriter")
			u.HostCalls = []HostCallRule{{Package: "os", Function: "OpenFile", Method: "openAST"}, {Package: "os", Function: "Getwd", Method: "astWorkingDirectory"}}
			u.Checks = map[string]string{"NewHTMLWriter": "checkHTMLWriter"}
		}
	}
	return units
}

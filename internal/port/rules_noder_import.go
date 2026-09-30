package port

func noderImportMigration(units []Unit) []Unit {
	const root = "src/cmd/compile/internal/"
	// Select the upstream portable read implementation explicitly. The library
	// owns no process-lifetime mmap and accepts seekable in-memory archives.
	units = append(units, Unit{Source: root + "base/mapfile_read.go", BuildVariant: "!unix", Functions: []string{"MapFile"}, HostSignatures: []HostSignatureRule{{Function: "MapFile", Methods: []string{"ReadAt"}}}})
	methods := []string{"Read", "ReadAt", "Seek", "Close", "Name"}
	units = append(units, Unit{Source: root + "noder/import.go", WholeFile: true,
		Context:        &ContextRule{Type: "Context", Constructor: "initContext", Initialize: true, PreserveNilArgs: true, RedirectTrace: true, Adapter: "adapter_context.go", Selective: true},
		Owner:          &OwnerRule{Types: []string{"gcimports"}, Context: "Context", Field: "compilation"},
		HostSignatures: []HostSignatureRule{{Function: "readExportData", Methods: methods}, {Function: "openPackage", Result: true, Methods: methods}},
		HostConstants:  []HostConstantRule{{Package: "runtime", Constant: "GOOS", Field: "hostOS"}},
		HostCalls:      []HostCallRule{{Package: "os", Function: "Open", Method: "openDependency", StreamMethods: methods}},
		State: []StateRule{
			{Source: root + "base/link.go", Variable: "Ctxt", Field: "Ctxt"},
			{Source: root + "base/link.go", Variable: "ReservedImports", Field: "Base.ReservedImports"},
			{Source: root + "base/flag.go", Variable: "Flag", Field: "Base.Flag"},
			{Source: root + "base/debug.go", Variable: "Debug", Field: "Typecheck.IR.Debug"},
			{Source: root + "typecheck/target.go", Variable: "Target", Field: "Typecheck.Target"},
			{Source: root + "types/fmt.go", Variable: "UnsafePkg", Field: "Types.UnsafePkg"},
			{Source: root + "types2/universe.go", Variable: "Unsafe", Field: "Checker.Unsafe"},
			{Source: "src/internal/buildcfg/cfg.go", Variable: "GOOS", Field: "GOOS"},
			{Source: "src/internal/buildcfg/cfg.go", Variable: "GOARCH", Field: "GOARCH"},
			{Source: "src/internal/buildcfg/cfg.go", Variable: "GOROOT", Field: "GOROOT"},
		},
		Calls: []CallRule{
			{Source: root + "base/print.go", Function: "Fatalf", Method: "fatalf"},
			{Source: root + "types/pkg.go", Function: "NewPkg", Method: "Types.NewPkg"},
			{Source: root + "importer/ureader.go", Function: "ReadPackage", Method: "typeImports.ReadPackage"},
			{Source: "src/cmd/internal/objabi/util.go", Function: "HeaderString", Method: "importHeader.HeaderString"},
		},
	})
	var out []Unit
	for _, pkg := range []string{"internal/saferio", "internal/exportdata"} {
		for _, u := range units {
			if u.sourcePackage() == pkg {
				out = append(out, u)
			}
		}
	}
	for _, u := range units {
		if u.sourcePackage() != "internal/saferio" && u.sourcePackage() != "internal/exportdata" {
			out = append(out, u)
		}
	}
	return out
}

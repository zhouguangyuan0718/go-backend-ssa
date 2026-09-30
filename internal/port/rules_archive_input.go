package port

// archiveInputMigration preserves native buffered seeking and archive parsing.
// Callers supply streams; filesystem discovery and mmap are outside this stage.
func archiveInputMigration(units []Unit) []Unit {
	for i := range units {
		u := &units[i]
		if u.Source != "src/cmd/internal/bio/buf.go" {
			continue
		}
		u.Functions = append(u.Functions, "NewReader")
		u.Declarations = append(u.Declarations, "Reader", "Reader.MustSeek", "Reader.Offset", "Reader.Close", "Reader.File")
		methods := []string{"Read", "Seek", "Close", "ReadAt"}
		u.HostFields = append(u.HostFields, HostFieldRule{Type: "Reader", Field: "f", Methods: methods})
		u.HostSignatures = append(u.HostSignatures,
			HostSignatureRule{Function: "NewReader", Index: 0, Methods: methods},
			HostSignatureRule{Function: "Reader.File", Result: true, Index: 0, Methods: methods})
		for _, name := range []string{"Reader.MustSeek", "Reader.Offset"} {
			u.ContextFrom[name] = ContextSource{Argument: "r", Resolver: "readerContext"}
		}
	}
	return append(units,
		Unit{Source: "src/internal/saferio/io.go", Functions: []string{"ReadData", "ReadDataAt"}, Declarations: []string{"chunk"}},
		Unit{Source: "src/internal/exportdata/support.go", Functions: []string{"readArchiveHeader"}},
		Unit{Source: "src/internal/exportdata/exportdata.go", Functions: []string{"ReadUnified", "FindPackageDefinition", "ReadObjectHeaders", "ReadExportDataHeader"}},
	)
}

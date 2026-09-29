package port

// Keep pkgbits' native encoder/decoder instances. Only the decoder's diagnostic
// sink, failure boundary and overflow sentinel need adaptation.
func pkgbitsMigration() []Unit {
	context := func(fields map[string]string) *ContextRule {
		return &ContextRule{Existing: true, Selective: true, Type: "PkgDecoder", Constructor: "NewPkgDecoder", Adapter: "adapter_decoder.go", Embed: "decoderIO", RedirectTrace: true, RedirectExit: true, Fields: fields}
	}
	return []Unit{
		{Source: "src/internal/pkgbits/codes.go", WholeFile: true, Context: context(nil)},
		{Source: "src/internal/pkgbits/decoder.go", WholeFile: true, Context: context(map[string]string{"overflow": "overflow"}), Owner: &OwnerRule{Existing: true, Types: []string{"Decoder"}, Context: "PkgDecoder", Field: "common"}},
		{Source: "src/internal/pkgbits/doc.go", WholeFile: true, Context: context(nil)},
		{Source: "src/internal/pkgbits/encoder.go", WholeFile: true, Context: context(nil)},
		{Source: "src/internal/pkgbits/flags.go", WholeFile: true, Context: context(nil)},
		{Source: "src/internal/pkgbits/reloc.go", WholeFile: true, Context: context(nil)},
		{Source: "src/internal/pkgbits/support.go", WholeFile: true, Context: context(nil)},
		{Source: "src/internal/pkgbits/sync.go", WholeFile: true, Context: context(nil)},
		{Source: "src/internal/pkgbits/syncmarker_string.go", WholeFile: true, Context: context(nil), ReadTables: []string{"_SyncMarker_index"}},
		{Source: "src/internal/pkgbits/version.go", WholeFile: true, Context: context(nil), ReadTables: []string{"introduced", "removed"}},
	}
}

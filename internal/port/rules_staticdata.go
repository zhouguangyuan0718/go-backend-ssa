package port

func staticdataMigration() []Unit {
	u := Unit{
		Source: "src/cmd/compile/internal/staticdata/data.go",
		// Native file reads use a caller-owned fs.File provider.
		Functions:    []string{"InitAddrOffset", "InitAddr", "InitSlice", "InitSliceBytes", "shortHashString", "StringSym", "StringSymNoCommon", "slicedata", "dstringdata", "FuncLinksym", "GlobalLinksym", "WriteFuncSyms", "InitConst", "fileStringSym"},
		Declarations: []string{"stringSymPrefix", "stringSymPattern", "maxFileSize"},
		Context: &ContextRule{Type: "Context", Constructor: "newContext", Adapter: "adapter_context.go", Selective: true, Fields: map[string]string{
			"slicedataGen": "slicedataGen", "funcsymsmu": "funcsymsmu", "funcsyms": "funcsyms",
		}},
		State: []StateRule{
			{Source: "src/cmd/compile/internal/base/link.go", Variable: "Ctxt", Field: "Writer.Link"},
			{Source: "src/cmd/compile/internal/base/flag.go", Variable: "Flag", Field: "IR.Base.Flag"},
			{Source: "src/cmd/compile/internal/types/size.go", Variable: "PtrSize", Field: "ptrSize"},
			{Source: "src/cmd/compile/internal/types/size.go", Variable: "SliceLenOffset", Field: "IR.Types.SliceLenOffset"},
			{Source: "src/cmd/compile/internal/types/size.go", Variable: "SliceCapOffset", Field: "IR.Types.SliceCapOffset"},
			{Source: "src/cmd/compile/internal/types/fmt.go", Variable: "LocalPkg", Field: "IR.Types.LocalPkg"},
		},
		Calls: []CallRule{
			{Source: "src/cmd/compile/internal/base/print.go", Function: "Fatalf", Method: "fatalf"},
			{Source: "src/cmd/compile/internal/base/print.go", Function: "ErrorfAt", Method: "errorfAt"},
			{Source: "src/cmd/compile/internal/objw/objw.go", Function: "Global", Method: "Writer.Global"},
			{Source: "src/cmd/compile/internal/objw/objw.go", Function: "SymPtr", Method: "Writer.SymPtr"},
			{Source: "src/cmd/compile/internal/ir/val.go", Function: "IntVal", Method: "IR.IntVal"},
		},
		HostCalls: []HostCallRule{{Package: "os", Function: "Open", Method: "openFile"}},
		Checks:    map[string]string{"InitAddrOffset": "checkAddr", "InitSlice": "checkSlice", "InitSliceBytes": "checkSliceBytes", "FuncLinksym": "checkName", "GlobalLinksym": "checkName", "InitConst": "checkConst"},
	}
	embed := u
	embedContext := *u.Context
	embedContext.Fields = nil
	embed.Context = &embedContext
	embed.Source = "src/cmd/compile/internal/staticdata/embed.go"
	embed.WholeFile = true
	embed.Functions, embed.Declarations, embed.HostCalls = nil, nil, nil
	embed.Checks = map[string]string{"WriteEmbed": "checkEmbed"}
	embed.Calls = append(append([]CallRule{}, u.Calls...), CallRule{Source: "src/cmd/compile/internal/base/print.go", Function: "Errors", Method: "errors"}, CallRule{Source: "src/cmd/compile/internal/objw/objw.go", Function: "Uintptr", Method: "Writer.Uintptr"})
	return []Unit{u, embed}
}

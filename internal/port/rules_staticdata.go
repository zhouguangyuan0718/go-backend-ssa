package port

func staticdataMigration() []Unit {
	u := Unit{
		Source: "src/cmd/compile/internal/staticdata/data.go",
		// fileStringSym additionally needs explicit host file inputs and object
		// file streaming; do not import its process filesystem dependency here.
		Functions:    []string{"InitAddrOffset", "InitAddr", "InitSlice", "InitSliceBytes", "shortHashString", "StringSym", "StringSymNoCommon", "slicedata", "dstringdata", "FuncLinksym", "GlobalLinksym", "WriteFuncSyms", "InitConst"},
		Declarations: []string{"stringSymPrefix", "stringSymPattern"},
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
		Checks: map[string]string{"InitAddrOffset": "checkAddr", "InitSlice": "checkSlice", "InitSliceBytes": "checkSliceBytes", "FuncLinksym": "checkName", "GlobalLinksym": "checkName", "InitConst": "checkConst"},
	}
	return []Unit{u}
}

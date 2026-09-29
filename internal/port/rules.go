package port

// Baseline pins the official https://go.googlesource.com/go source.
// Return fresh values so upgrade tests can change the pin without shared state.
func Baseline() Pin {
	return Pin{
		Tag:            "go1.27.1",
		Commit:         "862c888e612ac346c7c4d99c9392bdfd265f33b0",
		BuildToolchain: "go1.27.1",
		TargetModule:   "go-backend-ssa",
	}
}

// Migration defines the selected source and the adaptations maintained here.
// Paths/imports are derived; package names come from the upstream AST.
// InspectRoots cover candidate dependencies beyond the currently ported files.
func Migration() Plan {
	return Plan{
		InspectRoots: []string{
			"cmd/compile/internal/gc",
			"cmd/compile/internal/syntax",
			"cmd/compile/internal/types2",
			"cmd/compile/internal/noder",
			"cmd/compile/internal/ir",
			"cmd/compile/internal/types",
			"cmd/compile/internal/typecheck",
			"cmd/compile/internal/ssagen",
			"cmd/compile/internal/ssa",
		},
		Units: []Unit{
			{Source: "src/cmd/internal/src/pos.go", WholeFile: true, ZeroValues: []string{"NoPos", "noPos"}},
			{Source: "src/cmd/internal/src/xpos.go", WholeFile: true, ZeroValues: []string{"NoXPos"}},
			{Source: "src/cmd/compile/internal/syntax/pos.go", WholeFile: true},
			{Source: "src/cmd/compile/internal/syntax/source.go", WholeFile: true},
			{
				Source:    "src/cmd/internal/objabi/path.go",
				Functions: []string{"PathToPrefix", "PrefixToPath"},
			},
			{Source: "src/cmd/internal/objabi/symkind.go", Declarations: []string{"SymKind", "Sxxx"}},
			{Source: "src/cmd/internal/objabi/reloctype.go", Declarations: []string{"RelocType"}},
			{Source: "src/cmd/internal/obj/link.go", Declarations: []string{"LSym", "Attribute", "Reloc"}},
			{
				Source:    "src/cmd/compile/internal/ir/func.go",
				Functions: []string{"ParseLinkFuncName", "splitPkg", "escapedImportPathOK", "modPathOK"},
			},
			{Source: "src/cmd/compile/internal/types/utils.go", WholeFile: true},
			{Source: "src/cmd/compile/internal/types/kind_string.go", WholeFile: true, ReadTables: []string{"_Kind_index"}},
			{Source: "src/cmd/compile/internal/types/alg.go", Declarations: []string{"AlgKind", "AUNK"}},
			{Source: "src/cmd/compile/internal/types/type.go",
				Functions: []string{"newType", "NewTuple", "newSSA", "newSIMD", "newResults", "NewResults", "NewPtr", "NewSlice", "NewArray", "NewMap", "NewChan"},
				Declarations: []string{
					"Object", "Kind", "Txxx", "ChanDir", "Crecv", "Type", "typeNotInHeap",
					"Map", "Forward", "Func", "Struct", "Interface", "Ptr", "ChanArgs", "FuncArgs", "Chan", "Tuple", "Results", "Array", "Slice", "Field", "fields", "fieldIsDDD",
					"Type.wantEtype", "Type.Elem", "Type.Key", "Type.MapType", "Type.chanType", "Type.NumElem", "Type.ChanDir", "Type.PtrTo",
					"Type.IsPtr", "Type.IsSlice", "Type.IsArray", "Type.IsMap", "Type.IsChan", "Type.IsPtrShaped", "Type.HasNil",
					"Type.IsMemory", "Type.IsFlags", "Type.IsVoid", "Type.IsTuple", "Type.IsResults", "Type.IsSIMD",
					"Type.Kind", "Type.Underlying", "Type.Sym", "Type.CanBeAnSSAAux", "Type.HasShape", "Type.SetHasShape", "Type.IsShape", "Type.SetIsShape", "Type.NotInHeap", "Type.SetNotInHeap", "Type.Noalg", "Type.SetNoalg",
					"fields.Slice", "fields.Set", "ChanDir.CanRecv", "ChanDir.CanSend",
				},
				Calls: []CallRule{{Source: "src/cmd/compile/internal/base/print.go", Function: "Fatalf", Method: "fatalf"}},
				Owner: &OwnerRule{Types: []string{"Type"}, Context: "Universe", Field: "universe"},
				Context: &ContextRule{
					Type: "Universe", Constructor: "newUniverse", Adapter: "adapter_context.go", Init: true,
					Fields: map[string]string{
						"TypeInvalid": "TypeInvalid", "TypeMem": "TypeMem", "TypeFlags": "TypeFlags", "TypeVoid": "TypeVoid",
						"TypeInt128": "TypeInt128", "TypeVec128": "TypeVec128", "TypeVec256": "TypeVec256", "TypeVec512": "TypeVec512",
						"TypeMask": "TypeMask", "TypeResultMem": "TypeResultMem",
						"NewPtrCacheEnabled": "NewPtrCacheEnabled",
					},
				},
			},
			{Source: "src/cmd/compile/internal/types/size.go",
				Context: &ContextRule{Type: "Universe", Constructor: "newUniverse", Adapter: "adapter_context.go", Fields: map[string]string{"PtrSize": "ptrSize"}},
			},
			{Source: "src/cmd/compile/internal/types/sym.go", Functions: []string{"IsExported", "CompareSyms"},
				Declarations: []string{"Sym", "symOnExportList", "Sym.IsBlank", "Sym.OnExportList", "Sym.Uniq", "Sym.Siggen", "Sym.Asm", "Sym.Func", "Sym.SetOnExportList", "Sym.SetUniq", "Sym.SetSiggen", "Sym.SetAsm", "Sym.SetFunc"},
			},
			{
				Source:       "src/cmd/compile/internal/types/pkg.go",
				Functions:    []string{"InternString", "NewPkg"},
				Declarations: []string{"Pkg", "Pkg.Lookup", "Pkg.LookupOK", "Pkg.LookupBytes", "Pkg.LookupNum"},
				Owner:        &OwnerRule{Types: []string{"Pkg"}, Context: "Universe", Field: "universe"},
				Context: &ContextRule{
					Type: "Universe", Constructor: "newUniverse",
					Adapter: "adapter_context.go",
					Fields: map[string]string{
						"internedStringsmu": "mu",
						"internedStrings":   "strings",
						"pkgMap":            "packages",
						"nopkg":             "anonymous",
					},
				},
			},
		},
	}
}

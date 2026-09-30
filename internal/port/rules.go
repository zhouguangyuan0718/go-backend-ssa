package port

import "strings"

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
	plan := Plan{
		SupportFiles: []string{"position/position.go", "diagnostic/diagnostic.go"},
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
			{Source: "src/internal/abi/symtab.go", WholeFile: true},
			{Source: "src/cmd/internal/src/pos.go", WholeFile: true, ZeroValues: []string{"NoPos", "noPos"}},
			{Source: "src/cmd/internal/src/xpos.go", WholeFile: true, ZeroValues: []string{"NoXPos"}},
			{Source: "src/cmd/compile/internal/syntax/pos.go", WholeFile: true},
			{Source: "src/cmd/compile/internal/syntax/source.go", WholeFile: true},
			{Source: "src/cmd/compile/internal/syntax/syntax.go", Functions: []string{"Parse"}, Declarations: []string{"Mode", "CheckBranches", "Error", "Error.Error", "ErrorHandler", "Pragma", "PragmaHandler"}},
			{Source: "src/cmd/compile/internal/syntax/nodes.go", WholeFile: true},
			{Source: "src/cmd/compile/internal/syntax/type.go", WholeFile: true},
			{Source: "src/cmd/compile/internal/syntax/tokens.go", WholeFile: true},
			{Source: "src/cmd/compile/internal/syntax/token_string.go", WholeFile: true, ReadTables: []string{"_token_index"}},
			{Source: "src/cmd/compile/internal/syntax/operator_string.go", WholeFile: true, ReadTables: []string{"_Operator_index"}},
			{Source: "src/cmd/compile/internal/syntax/parser.go", WholeFile: true},
			{Source: "src/cmd/compile/internal/syntax/scanner.go", WholeFile: true, ReceiverState: &ReceiverStateRule{Type: "scanner", InitMethod: "init", Fields: map[string]string{"keywordMap": "keywordMap"}, RunInit: true}},
			{Source: "src/cmd/compile/internal/syntax/branches.go", WholeFile: true, ReceiverState: &ReceiverStateRule{Type: "labelScope", Fields: map[string]string{"invalid": "invalid"}}},
			{Source: "src/cmd/compile/internal/syntax/printer.go", WholeFile: true, ReceiverState: &ReceiverStateRule{Type: "printer", Fields: map[string]string{"tabBytes": "tabBytes", "newlineByte": "newlineByte", "blankByte": "blankByte"}}},
			{Source: "src/cmd/compile/internal/syntax/dumper.go", WholeFile: true, ReceiverState: &ReceiverStateRule{Type: "dumper", Fields: map[string]string{"indentBytes": "indentBytes"}}},
			{Source: "src/cmd/compile/internal/syntax/positions.go", WholeFile: true},
			{Source: "src/cmd/compile/internal/syntax/walk.go", WholeFile: true},
			{Source: "src/cmd/compile/internal/syntax/testing.go", WholeFile: true},
			{
				Source:    "src/cmd/internal/objabi/path.go",
				Functions: []string{"PathToPrefix", "PrefixToPath"},
			},
			{Source: "src/cmd/internal/objabi/line.go", Functions: []string{"AbsFile", "ApplyRewrites", "applyRewrite", "hasPathPrefix"},
				Context: &ContextRule{Type: "Context", Constructor: "newContext", Adapter: "adapter_path.go", Selective: true},
				State:   []StateRule{{Source: "src/internal/buildcfg/cfg.go", Variable: "GOROOT", Field: "GOROOT"}}},
			{Source: "src/cmd/internal/objabi/pkgspecial.go", WholeFile: true,

				Context: &ContextRule{Type: "Context", Constructor: "newContext", Adapter: "adapter_path.go", Selective: true, Fields: map[string]string{"pkgSpecialsOnce": "pkgSpecialsOnce", "runtimePkgs": "runtimePkgs", "extraNoInstrumentPkgs": "extraNoInstrumentPkgs", "noRaceFuncPkgs": "noRaceFuncPkgs", "allowAsmABIPkgs": "allowAsmABIPkgs"}}},
			{Source: "src/cmd/internal/objabi/util.go", Declarations: []string{"GlobalDictPrefix"}},
			{Source: "src/cmd/internal/objabi/symkind.go", WholeFile: true},
			{Source: "src/cmd/internal/objabi/reloctype.go", WholeFile: true},
			{Source: "src/cmd/internal/obj/link.go", Declarations: []string{"Addr", "AddrName", "AddrPos", "AddrType", "As", "Attribute", "Func", "LSym", "Link", "LinkArch", "OperandPos", "Prog", "Reloc", "ABI", "ABI0", "ABISet", "ABISetCallable", "AttrDuplicateOK", "symOnce", "Attribute.load", "Attribute.Set", "Attribute.ABI", "Attribute.SetABI", "Attribute.Static", "ABISet.Set", "ABISet.Get", "ABISet.String", "Link.Diag", "Attribute.OnList", "Attribute.DuplicateOK", "Attribute.Local", "Attribute.IsLinkname", "Attribute.IsLinknameStd", "LSym.String", "LSym.NewTypeInfo", "LSym.TypeInfo", "LSym.NewItabInfo", "LSym.ItabInfo", "TypeInfo", "ItabInfo", "FuncInfo", "LSym.NewFuncInfo", "LSym.Func", "Pcln", "InlMark", "RegSpill", "JumpTable", "WasmImport", "WasmExport", "WasmFuncType", "WasmField", "WasmFieldType", "Attribute.WasInlined", "Link.Logf"}, Functions: []string{"ABISetOf", "ParseABI"}, Context: objectDataContext()},
			{Source: "src/internal/goexperiment/flags.go", Declarations: []string{"Flags"}},
			{Source: "src/internal/buildcfg/exp.go", Declarations: []string{"ExperimentFlags"}},
			{Source: "src/internal/types/errors/codes.go", WholeFile: true},
			{Source: "src/cmd/compile/internal/base/base.go", Declarations: []string{"EnableTrace"}},
			{Source: "src/cmd/compile/internal/types/utils.go", WholeFile: true},
			{Source: "src/cmd/compile/internal/types/kind_string.go", WholeFile: true, ReadTables: []string{"_Kind_index"}},
			{Source: "src/cmd/compile/internal/types/algkind_string.go", WholeFile: true, ReadTables: []string{"_AlgKind_index"}},
			{Source: "src/cmd/compile/internal/types/alg.go", Declarations: []string{"AlgKind", "AUNK", "Type.setAlg"},
				Functions: []string{"AlgType", "TypeHasNoAlg", "IsComparable", "IncomparableField", "IsPaddedField"},
				Context:   &ContextRule{Type: "Universe", Constructor: "newUniverse", Adapter: "adapter_context.go", Fields: map[string]string{"algPriority": "algPriority"}},
				Calls:     []CallRule{{Source: "src/cmd/compile/internal/base/print.go", Function: "Fatalf", Method: "fatalf"}},
			},
			{Source: "src/cmd/compile/internal/types/universe.go", Functions: []string{"InitTypes", "makeErrorInterface", "makeComparableInterface"},
				Context: &ContextRule{Type: "Universe", Constructor: "newUniverse", Adapter: "adapter_context.go", Fields: map[string]string{"basicTypes": "basicTypes", "typedefs": "typedefs"}},
				Calls:   []CallRule{{Source: "src/cmd/compile/internal/base/print.go", Function: "Fatalf", Method: "fatalf"}},
				Checks:  map[string]string{"InitTypes": "checkInitTypes"},
			},
			{Source: "src/cmd/compile/internal/types/identity.go", Declarations: []string{"identIgnoreTags", "typePair"},
				Functions: []string{"Identical", "IdenticalIgnoreTags", "IdenticalStrict", "identical"},
				Nullable:  map[string][]string{"Identical": {"t1", "t2"}, "IdenticalIgnoreTags": {"t1", "t2"}, "IdenticalStrict": {"t1", "t2"}, "identical": {"t1", "t2"}},
				Context:   &ContextRule{Type: "Universe", Constructor: "newUniverse", Adapter: "adapter_context.go"},
			},
			{Source: "src/cmd/compile/internal/types/fmt.go", Declarations: []string{"fmtMode", "fmtGo", "Sym.Format", "Sym.String", "Type.Format", "Type.String", "Type.LinkString", "Type.NameString"},
				ContextFrom: map[string]ContextSource{"Sym.Format": {Argument: "s", Resolver: "formatUniverse"}, "Sym.String": {Argument: "s", Resolver: "formatUniverse"}},
				Functions:   []string{"sconv", "sconv2", "symfmt", "pkgqual", "tconv", "tconv2", "formatParams", "fldconv", "SplitVargenSuffix", "SplitMethSuffix", "TypeHash"},
				Nullable:    map[string][]string{"tconv": {"t"}, "tconv2": {"t"}, "pkgqual": {"pkg"}, "Type.Format": {"t"}, "Type.String": {"t"}, "Type.LinkString": {"t"}, "Type.NameString": {"t"}},
				Context: &ContextRule{Type: "Universe", Constructor: "newUniverse", Adapter: "adapter_context.go", Fields: map[string]string{
					"BuiltinPkg": "BuiltinPkg", "LocalPkg": "LocalPkg", "UnsafePkg": "UnsafePkg", "BlankSym": "BlankSym", "NumImport": "NumImport", "BasicTypeNames": "BasicTypeNames", "fmtBufferPool": "fmtBufferPool",
				}},
				Calls: []CallRule{{Source: "src/cmd/compile/internal/base/print.go", Function: "Fatalf", Method: "fatalf"}, {Source: "src/cmd/compile/internal/base/print.go", Function: "Assertf", Method: "assertf"}},
			},
			{Source: "src/cmd/compile/internal/types/type.go",
				ContextAfterGuards: map[string]int{"Type.cmp": 3},
				StatelessFunctions: []string{"cmpForNe"},
				Functions:          []string{"cmpForNe", "IsReflexive", "IsDirectIface", "TypeSym", "TypeSymLookup", "TypeSymName", "IsInterfaceMethod", "ComplexForFloat", "ReceiverBaseType", "RuntimeSymName", "ReflectSymName", "IsNoInstrumentPkg", "IsNoRacePkg", "IsRuntimePkg", "IsMethodApplicable", "SubstAny", "substFields", "FloatForComplex", "newType", "NewTuple", "newSSA", "newSIMD", "newResults", "NewResults", "NewPtr", "NewSlice", "NewArray", "NewMap", "NewChan", "NewField", "NewStruct", "NewSignature", "fieldsHasShape", "CompareFields", "NewChanArgs", "NewFuncArgs", "NewInterface", "FakeRecv", "FakeRecvType", "NewNamed"},
				Declarations: []string{
					"Cmp", "CMPlt", "Type.Compare", "Type.cmp", "Sym.cmpsym", "Object", "Kind", "Txxx", "ChanDir", "Crecv", "Type", "typeNotInHeap",
					"Map", "Forward", "Func", "Struct", "Interface", "Ptr", "ChanArgs", "FuncArgs", "Chan", "Tuple", "Results", "Array", "Slice", "Field", "fields", "fieldIsDDD",
					"Type.SoleComponent", "Type.ToUnsigned", "Type.IsPtrElem", "Type.IsFullyInstantiated", "Type.SetIsFullyInstantiated", "Type.IsUintptr", "Type.copy", "Type.forwardType", "Type.SetUnderlying", "Type.Obj", "Type.IsUnsafePtr", "Field.End", "Type.Pos", "Type.Registers", "Type.Deferwidth", "Type.SetDeferwidth", "Type.Recur", "Type.SetRecur",
					"Type.ChanArgs", "Type.FuncArgs", "Type.Methods", "Type.AllMethods", "Type.SetMethods", "Type.SetAllMethods", "Type.SetInterface", "Type.IsInterface", "Type.IsEmptyInterface", "Type.Size", "Type.Alignment", "Type.ArgWidth", "Type.HasPointers",
					"Field.IsDDD", "Field.Nointerface", "Field.SetIsDDD", "Field.SetNointerface", "Field.Copy", "Field.IsMethod",
					"Type.OffsetOf", "Type.StructType", "Type.fields", "Type.Fields", "Type.Field", "Type.setFields", "Type.IsStruct", "Type.IsFuncArgStruct", "Type.NumFields", "Type.FieldType",
					"Type.funcType", "Func.recvs", "Func.params", "Func.results", "Func.recvParams", "Func.paramsResults",
					"Type.ResultsTuple", "Type.Recvs", "Type.Params", "Type.Results", "Type.RecvParamsResults", "Type.RecvParams", "Type.ParamsResults",
					"Type.NumRecvs", "Type.NumParams", "Type.NumResults", "Type.IsVariadic", "Type.Recv", "Type.Param", "Type.Result",
					"Type.wantEtype", "Type.Elem", "Type.Key", "Type.MapType", "Type.chanType", "Type.NumElem", "Type.ChanDir", "Type.PtrTo",
					"Type.IsPtr", "Type.IsSlice", "Type.IsArray", "Type.IsMap", "Type.IsChan", "Type.IsPtrShaped", "Type.HasNil",
					"Type.IsKind", "Type.IsBoolean", "Type.IsInteger", "Type.IsScalar", "Type.IsSigned", "Type.IsUnsigned", "Type.IsFloat", "Type.IsComplex", "Type.IsString", "Type.IsUntyped",
					"Type.IsMemory", "Type.IsFlags", "Type.IsVoid", "Type.IsTuple", "Type.IsResults", "Type.IsSIMD",
					"Type.Kind", "Type.Underlying", "Type.Sym", "Type.CanBeAnSSAAux", "Type.HasShape", "Type.SetHasShape", "Type.IsShape", "Type.SetIsShape", "Type.NotInHeap", "Type.SetNotInHeap", "Type.Noalg", "Type.SetNoalg",
					"fields.Slice", "fields.Set", "ChanDir.CanRecv", "ChanDir.CanSend",
				},
				Nullable: map[string][]string{"ReceiverBaseType": {"t"}, "SubstAny": {"t"}, "Type.copy": {"t"}},
				Checks:   map[string]string{"SubstAny": "checkSubstitution", "NewField": "checkNewField", "NewStruct": "checkFields", "NewSignature": "checkSignature", "Type.setFields": "checkFields", "NewInterface": "checkMethods", "Type.SetInterface": "checkMethods", "Type.SetMethods": "checkMethods", "Type.SetAllMethods": "checkMethods", "NewNamed": "checkNamed"},
				Calls:    []CallRule{{Source: "src/cmd/internal/objabi/pkgspecial.go", Function: "LookupPkgSpecial", Method: "pkgSpecials.LookupPkgSpecial"}, {Source: "src/cmd/compile/internal/base/print.go", Function: "Fatalf", Method: "fatalf"}, {Source: "src/cmd/compile/internal/base/print.go", Function: "ErrorfAt", Method: "errorfAt"}},
				State:    []StateRule{{Source: "src/cmd/compile/internal/base/print.go", Variable: "AutogeneratedPos", Field: "AutogeneratedPos"}},
				Owner:    &OwnerRule{Types: []string{"Type"}, Context: "Universe", Field: "universe"},
				Context: &ContextRule{
					Type: "Universe", Constructor: "newUniverse", Adapter: "adapter_context.go", Init: true,
					Fields: map[string]string{
						"unsignedEType": "unsignedEType", "typepkgmu": "typepkgmu", "typepkg": "typepkg",
						"TypeInvalid": "TypeInvalid", "TypeMem": "TypeMem", "TypeFlags": "TypeFlags", "TypeVoid": "TypeVoid",
						"TypeInt128": "TypeInt128", "TypeVec128": "TypeVec128", "TypeVec256": "TypeVec256", "TypeVec512": "TypeVec512",
						"TypeMask": "TypeMask", "TypeResultMem": "TypeResultMem",
						"NewPtrCacheEnabled": "NewPtrCacheEnabled",
						"Types":              "Types", "ByteType": "ByteType", "RuneType": "RuneType", "AnyType": "AnyType", "ErrorType": "ErrorType", "SimType": "SimType",
						"UntypedString": "UntypedString", "UntypedBool": "UntypedBool",
						"UntypedInt": "UntypedInt", "UntypedRune": "UntypedRune", "UntypedFloat": "UntypedFloat", "UntypedComplex": "UntypedComplex",
						"UntypedTypes": "UntypedTypes", "DefaultKinds": "DefaultKinds",
						"ComparableType": "ComparableType", "recvType": "recvType", "ShapePkg": "ShapePkg", "IsOrdered": "IsOrdered", "IsSimple": "IsSimple", "IsInt": "IsInt", "IsFloat": "IsFloat", "IsComplex": "IsComplex",
					},
				},
			},
			{Source: "src/cmd/compile/internal/types/size.go",
				Declarations: []string{"Type.widthCalculated"},
				Functions:    []string{"typePos", "RoundUp", "expandiface", "calcStructOffset", "isAtomicStdPkg", "CalcSize", "simdify", "CalcStructSize", "CalcArraySize", "CheckSize", "DeferCheckSize", "ResumeCheckSize", "PtrDataSize"},
				Nullable:     map[string][]string{"CalcSize": {"t"}, "CheckSize": {"t"}},
				Context: &ContextRule{Type: "Universe", Constructor: "newUniverse", Adapter: "adapter_context.go", Fields: map[string]string{
					"PtrSize": "ptrSize", "RegSize": "regSize", "MaxWidth": "maxWidth", "SlicePtrOffset": "SlicePtrOffset", "SliceLenOffset": "SliceLenOffset", "SliceCapOffset": "SliceCapOffset", "SliceSize": "SliceSize", "StringSize": "StringSize", "SkipSizeForTracing": "SkipSizeForTracing", "CalcSizeDisabled": "CalcSizeDisabled", "defercalc": "defercalc", "deferredTypeStack": "deferredTypeStack",
				}},
				Calls: []CallRule{{Source: "src/cmd/compile/internal/base/print.go", Function: "Fatalf", Method: "fatalf"}, {Source: "src/cmd/compile/internal/base/print.go", Function: "Errorf", Method: "errorf"}, {Source: "src/cmd/compile/internal/base/print.go", Function: "ErrorfAt", Method: "errorfAt"}},
				State: []StateRule{{Source: "src/cmd/compile/internal/base/print.go", Variable: "Pos", Field: "Pos"}, {Source: "src/internal/buildcfg/exp.go", Variable: "Experiment", Field: "Experiment"}},
			},
			{Source: "src/cmd/compile/internal/types/sym.go",
				Context:      &ContextRule{Type: "Universe", Constructor: "newUniverse", Adapter: "adapter_context.go", Selective: true},
				ContextFrom:  map[string]ContextSource{"Sym.LinksymABI": {Argument: "sym", Resolver: "symbolUniverse"}},
				Calls:        []CallRule{{Source: "src/cmd/compile/internal/base/print.go", Function: "Fatalf", Method: "fatalf"}, {Source: "src/cmd/compile/internal/base/link.go", Function: "Linkname", Method: "links.Linkname"}, {Source: "src/cmd/compile/internal/base/link.go", Function: "PkgLinksym", Method: "links.PkgLinksym"}},
				Functions:    []string{"IsExported", "CompareSyms"},
				Declarations: []string{"Sym", "symOnExportList", "Sym.IsBlank", "Sym.OnExportList", "Sym.Uniq", "Sym.Siggen", "Sym.Asm", "Sym.Func", "Sym.SetOnExportList", "Sym.SetUniq", "Sym.SetSiggen", "Sym.SetAsm", "Sym.SetFunc", "Sym.Linksym", "Sym.LinksymABI"},
			},
			{
				Source:       "src/cmd/compile/internal/types/pkg.go",
				Functions:    []string{"InternString", "NewPkg", "PkgMap"},
				Declarations: []string{"Pkg", "Pkg.Lookup", "Pkg.LookupOK", "Pkg.LookupBytes", "Pkg.LookupNum", "Pkg.Selector"},
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
	for i, u := range plan.Units {
		if u.Source == "src/cmd/internal/obj/link.go" {
			units := append([]Unit(nil), plan.Units[:i]...)
			dataUnits := objectDataMigration()
			units = append(units, dataUnits[:1]...) // bisect precedes native Link state
			units = append(units, objectDeclarations()...)
			units = append(units, dataUnits[1:]...)
			plan.Units = append(units, plan.Units[i:]...)
			break
		}
	}
	plan.Units = append(plan.Units, linkMigration()...)
	plan.Units = append(plan.Units, irMigration()...)
	plan.Units = append(plan.Units, typecheckMigration()...)
	plan.Units = append(plan.Units, pgoMigration()...)
	plan.Units = append(plan.Units, logoptMigration()...)
	plan.Units = append(plan.Units, inlheurMigration()...)
	plan.Units = append(plan.Units, inlineMigration()...)
	plan.Units = append(plan.Units, devirtualizeMigration()...)
	plan.Units = append(plan.Units, interleavedMigration()...)
	plan.Units = append(plan.Units, loopvarMigration()...)
	plan.Units = append(plan.Units, types2Migration()...)
	plan.Units = append(plan.Units, pkgbitsMigration()...)
	plan.Units = append(plan.Units, rangefuncMigration()...)
	plan.Units = append(plan.Units, midwayMigration()...)
	plan.Units = append(plan.Units, objwMigration()...)
	plan.Units = append(plan.Units, staticdataMigration()...)
	plan.Units = append(plan.Units, runtimeABIMigration()...)
	plan.Units = append(plan.Units, rttypeMigration()...)
	plan.Units = append(plan.Units, typebitsMigration()...)
	plan.Units = append(plan.Units, compareMigration()...)
	plan.Units = append(plan.Units, reflectdataMigration()...)
	plan.Units = append(plan.Units, staticinitMigration()...)
	plan.Units = append(plan.Units, dwarfgenMigration()...)
	plan.Units = append(plan.Units, importerMigration()...)
	plan.Units = append(plan.Units, noderMigration()...)
	plan.Units = noderDiagnosticMigration(plan.Units)
	plan.Units = irDiagnosticMigration(plan.Units)
	plan.Units = noderSourceMigration(plan.Units)
	plan.Units = append(plan.Units, prepackageMigration()...)
	plan.Units = append(plan.Units, pkginitMigration()...)
	plan.Units = append(plan.Units, ssagenABIMigration()...)
	plan.Units = append(plan.Units, deadlocalsMigration()...)
	plan.Units = append(plan.Units, escapeMigration()...)
	plan.Units = append(plan.Units, sliceMigration()...)
	plan.Units = append(plan.Units, abiMigration()...)
	plan.Units = append(plan.Units, ssaObjectMigration()...)
	plan.Units = append(plan.Units, Unit{Source: "src/cmd/compile/internal/abt/avlint32.go", Declarations: []string{"T", "node32"}})
	plan.Units = append(plan.Units, Unit{Source: "src/internal/unsafeheader/unsafeheader.go", WholeFile: true})
	plan.Units = append(plan.Units, ssaValueMigration()...)
	plan.Units = append(plan.Units, walkMigration()...)
	plan.Units = append(plan.Units, ssaTypesMigration()...)
	plan.Units = append(plan.Units, ssaOpcodeMigration()...)
	plan.Units = append(plan.Units, ssaGraphMigration()...)
	plan.Units = ssaLoweringMigration(plan.Units)
	plan.Units = ssaGenericMigration(plan.Units)
	plan.Units = ssaDiagnosticMigration(plan.Units)
	plan.Units = ssaOptimizerMigration(plan.Units)
	plan.Units = ssaCallMigration(plan.Units)
	plan.Units = ssaAllocationMigration(plan.Units)
	plan.Units = ssaPhaseMigration(plan.Units)
	plan.Units = ssagenFrontendMigration(plan.Units)
	plan.Units = ssagenFrameMigration(plan.Units)
	plan.Units = ssaSourceDiagnosticMigration(plan.Units)
	plan.Units = ssagenBuildMigration(plan.Units)
	plan.Units = ssagenEmitMigration(plan.Units)
	plan.Units = amd64EmissionMigration(plan.Units)
	plan.Units = x86AssemblerMigration(plan.Units)
	plan.Units = arm64Migration(plan.Units)
	plan.Units = objectFlushMigration(plan.Units)
	plan.Units = objectFileMigration(plan.Units)
	plan.Units = archiveMigration(plan.Units)
	plan.Units = archiveInputMigration(plan.Units)
	plan.Units = noderImportMigration(plan.Units)
	plan.Units = gcCompileMigration(plan.Units)
	plan.Units = dataMigration(plan.Units)
	plan.Units = gcFixedPointMigration(plan.Units)
	for i := range plan.Units {
		if plan.Units[i].Source == "src/cmd/internal/obj/link.go" {
			plan.Units[i].Declarations = append(plan.Units[i].Declarations, "AXXX", "ABase386", "LSym.CanBeAnSSASym", "LSym.CanBeAnSSAAux")
		}
	}
	// base's symbol helpers now consume the native obj package. Keep their
	// entire package after obj/buildcfg and before the compiler types package.
	var baseUnits, units []Unit
	for _, u := range plan.Units {
		if strings.HasPrefix(u.Source, "src/cmd/compile/internal/base/") {
			baseUnits = append(baseUnits, u)
		}
	}
	for _, u := range plan.Units {
		if strings.HasPrefix(u.Source, "src/cmd/compile/internal/base/") {
			continue
		}
		if u.Source == "src/cmd/compile/internal/types/utils.go" {
			units = append(units, baseUnits...)
		}
		units = append(units, u)
	}
	plan.Units = units
	return plan
}

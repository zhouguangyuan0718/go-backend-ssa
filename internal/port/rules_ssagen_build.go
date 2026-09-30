package port

// Preserve the full IR builder, both native phi algorithms, and all intrinsic
// registration/lowering paths. Code emission remains a subsequent stage.
func ssagenBuildMigration(units []Unit) []Unit {
	const root = "src/cmd/compile/internal/"
	// Interface-switch cache policy now depends on the native architecture enum.
	for i, u := range units {
		if u.Source == "src/internal/goarch/goarch.go" {
			units = append([]Unit{u}, append(units[:i:i], units[i+1:]...)...)
			break
		}
	}

	units = append([]Unit{{Source: "src/internal/runtime/gc/sizeclasses.go", Declarations: []string{"SmallSizeDiv"}, Context: &ContextRule{Type: "Context", Constructor: "NewContext", Adapter: "adapter_context.go", Fields: map[string]string{"SizeToSizeClass8": "SizeToSizeClass8"}}}}, units...)
	var frame Unit
	for i := range units {
		u := &units[i]
		if u.Source == "src/internal/buildcfg/cfg.go" {
			u.Declarations = append(u.Declarations, "Goarm64Features")
		}
		if u.Source == "src/internal/abi/switch.go" {
			u.Functions = append(u.Functions, "UseInterfaceSwitchCache")
		}
		if u.Source == "src/cmd/internal/obj/link.go" {
			u.Declarations = append(u.Declarations, "Attribute.ABIWrapper")
		}
		if u.Source == root+"types/type.go" {
			u.Declarations = append(u.Declarations, "componentsIncludeBlankFields", "CountBlankFields", "Type.NumComponents")
		}
		if u.sourcePackage() == "cmd/compile/internal/ssagen" && u.Context != nil {
			u.Context.Constructor = "initialize"
			u.Context.Initialize = true
		}
		if u.Source == root+"ssagen/pgen.go" {
			frame = *u
		}
	}
	unique := func(a, b []string) []string {
		seen := map[string]bool{}
		var out []string
		for _, list := range [][]string{a, b} {
			for _, v := range list {
				if !seen[v] {
					seen[v] = true
					out = append(out, v)
				}
			}
		}
		return out
	}
	states := append([]StateRule{}, frame.State...)
	states = append(states, StateRule{Source: root + "types/type.go", Variable: "TypeInvalid", Field: "Typecheck.IR.Types.TypeInvalid"}, StateRule{Source: root + "rttype/rttype.go", Variable: "Type", Field: "SSA.ReflectData.Runtime.Type"}, StateRule{Source: root + "rttype/rttype.go", Variable: "ITab", Field: "SSA.ReflectData.Runtime.ITab"}, StateRule{Source: "src/internal/goexperiment/exp_runtimefreegc_off.go", Variable: "RuntimeFreegc", Field: "Typecheck.IR.Types.Experiment.RuntimeFreegc"})
	states = append(states, StateRule{Source: "src/internal/runtime/gc/sizeclasses.go", Variable: "SizeToSizeClass8", Field: "mallocGC.SizeToSizeClass8"})
	states = append(states, StateRule{Source: root + "types/type.go", Variable: "TypeMask", Field: "Typecheck.IR.Types.TypeMask"}, StateRule{Source: root + "types/fmt.go", Variable: "BlankSym", Field: "Typecheck.IR.Types.BlankSym"}, StateRule{Source: root + "types/size.go", Variable: "SliceLenOffset", Field: "Typecheck.IR.Types.SliceLenOffset"}, StateRule{Source: root + "types/size.go", Variable: "SliceCapOffset", Field: "Typecheck.IR.Types.SliceCapOffset"})
	calls := append([]CallRule{}, frame.Calls...)
	calls = append(calls, CallRule{Source: "src/cmd/internal/objabi/pkgspecial.go", Function: "LookupPkgSpecial", Method: "pkgSpecials.LookupPkgSpecial"})
	states = append(states, StateRule{Source: root + "base/hashdebug.go", Variable: "ConvertHash", Field: "Typecheck.IR.Base.ConvertHash"})
	states = append(states, StateRule{Source: root + "base/hashdebug.go", Variable: "VariableMakeHash", Field: "Typecheck.IR.Base.VariableMakeHash"})
	states = append(states, StateRule{Source: root + "types/type.go", Variable: "TypeMem", Field: "Typecheck.IR.Types.TypeMem"})
	states = append(states, StateRule{Source: root + "types/type.go", Variable: "TypeInt128", Field: "Typecheck.IR.Types.TypeInt128"})
	states = append(states, StateRule{Source: root + "types/type.go", Variable: "TypeVoid", Field: "Typecheck.IR.Types.TypeVoid"})
	states = append(states, StateRule{Source: root + "types/type.go", Variable: "TypeVec128", Field: "Typecheck.IR.Types.TypeVec128"})
	states = append(states, StateRule{Source: root + "types/type.go", Variable: "TypeVec256", Field: "Typecheck.IR.Types.TypeVec256"})
	states = append(states, StateRule{Source: root + "types/type.go", Variable: "TypeVec512", Field: "Typecheck.IR.Types.TypeVec512"})
	states = append(states, StateRule{Source: root + "ssa/compile.go", Variable: "IntrinsicsDebug", Field: "SSA.IntrinsicsDebug"})
	states = append(states, StateRule{Source: root + "ssa/compile.go", Variable: "IntrinsicsDisable", Field: "SSA.IntrinsicsDisable"})
	states = append(states, StateRule{Source: root + "ir/expr.go", Variable: "IsIntrinsicCall", Field: "Typecheck.IR.IsIntrinsicCall"})
	states = append(states, StateRule{Source: "src/internal/buildcfg/cfg.go", Variable: "GO386", Field: "GO386"})
	states = append(states, StateRule{Source: "src/internal/buildcfg/cfg.go", Variable: "GOAMD64", Field: "SSA.GOAMD64"})
	states = append(states, StateRule{Source: "src/internal/buildcfg/cfg.go", Variable: "GOARM", Field: "SSA.GOARM"})
	states = append(states, StateRule{Source: "src/internal/buildcfg/cfg.go", Variable: "GOARM64", Field: "GOARM64"})
	states = append(states, StateRule{Source: "src/internal/buildcfg/cfg.go", Variable: "GOMIPS", Field: "GOMIPS"})
	states = append(states, StateRule{Source: "src/internal/buildcfg/cfg.go", Variable: "GOMIPS64", Field: "GOMIPS64"})
	states = append(states, StateRule{Source: "src/internal/buildcfg/cfg.go", Variable: "GOOS", Field: "Typecheck.IR.Base.Ctxt.TargetOS"})
	states = append(states, StateRule{Source: "src/internal/buildcfg/cfg.go", Variable: "GOPPC64", Field: "SSA.GOPPC64"})
	states = append(states, StateRule{Source: "src/internal/buildcfg/cfg.go", Variable: "GORISCV64", Field: "SSA.GORISCV64"})
	states = append(states, StateRule{Source: "src/cmd/internal/sys/arch.go", Variable: "Archs", Field: "architectures.Archs"})
	states = append(states, StateRule{Source: "src/cmd/internal/sys/arch.go", Variable: "ArchAMD64", Field: "architectures.ArchAMD64"})
	states = append(states, StateRule{Source: "src/cmd/internal/sys/arch.go", Variable: "ArchARM64", Field: "architectures.ArchARM64"})
	states = append(states, StateRule{Source: "src/cmd/internal/sys/arch.go", Variable: "ArchLoong64", Field: "architectures.ArchLoong64"})
	states = append(states, StateRule{Source: "src/cmd/internal/sys/arch.go", Variable: "ArchPPC64", Field: "architectures.ArchPPC64"})
	states = append(states, StateRule{Source: "src/cmd/internal/sys/arch.go", Variable: "ArchPPC64LE", Field: "architectures.ArchPPC64LE"})
	states = append(states, StateRule{Source: "src/cmd/internal/sys/arch.go", Variable: "ArchRISCV64", Field: "architectures.ArchRISCV64"})
	states = append(states, StateRule{Source: "src/cmd/internal/sys/arch.go", Variable: "ArchWasm", Field: "architectures.ArchWasm"})
	calls = append(calls, CallRule{Source: "src/cmd/compile/internal/ir/fmt.go", Function: "DumpList", Method: "Typecheck.IR.DumpList"})
	calls = append(calls, CallRule{Source: "src/cmd/compile/internal/ir/fmt.go", Function: "FDumpList", Method: "Typecheck.IR.FDumpList"})
	calls = append(calls, CallRule{Source: "src/cmd/compile/internal/ir/stmt.go", Function: "NewReturnStmt", Method: "Typecheck.IR.NewReturnStmt"})
	calls = append(calls, CallRule{Source: "src/cmd/compile/internal/ir/val.go", Function: "IntVal", Method: "Typecheck.IR.IntVal"})
	calls = append(calls, CallRule{Source: "src/cmd/compile/internal/ir/val.go", Function: "Int64Val", Method: "Typecheck.IR.Int64Val"})
	calls = append(calls, CallRule{Source: "src/cmd/compile/internal/ir/val.go", Function: "BoolVal", Method: "Typecheck.IR.BoolVal"})
	calls = append(calls, CallRule{Source: "src/cmd/compile/internal/ir/val.go", Function: "StringVal", Method: "Typecheck.IR.StringVal"})
	calls = append(calls, CallRule{Source: "src/cmd/compile/internal/ir/expr.go", Function: "NewCallExpr", Method: "Typecheck.IR.NewCallExpr"})
	calls = append(calls, CallRule{Source: "src/cmd/compile/internal/ir/expr.go", Function: "SameSafeExpr", Method: "Typecheck.IR.SameSafeExpr"})
	calls = append(calls, CallRule{Source: "src/cmd/compile/internal/ir/expr.go", Function: "ShouldCheckPtr", Method: "Typecheck.IR.ShouldCheckPtr"})
	calls = append(calls, CallRule{Source: "src/cmd/compile/internal/ir/expr.go", Function: "IsReflectHeaderDataField", Method: "Typecheck.IR.IsReflectHeaderDataField"})
	calls = append(calls, CallRule{Source: "src/cmd/compile/internal/ir/expr.go", Function: "MethodExprName", Method: "Typecheck.IR.MethodExprName"})
	calls = append(calls, CallRule{Source: "src/cmd/compile/internal/ir/name.go", Function: "NewNameAt", Method: "Typecheck.IR.NewNameAt"})
	calls = append(calls, CallRule{Source: "src/cmd/compile/internal/ir/node.go", Function: "HasUniquePos", Method: "Typecheck.IR.HasUniquePos"})
	calls = append(calls, CallRule{Source: "src/cmd/compile/internal/types/size.go", Function: "RoundUp", Method: "Typecheck.IR.Types.RoundUp"})
	calls = append(calls, CallRule{Source: "src/cmd/compile/internal/types/size.go", Function: "CalcSize", Method: "Typecheck.IR.Types.CalcSize"})
	calls = append(calls, CallRule{Source: "src/cmd/compile/internal/types/size.go", Function: "CalcStructSize", Method: "Typecheck.IR.Types.CalcStructSize"})
	calls = append(calls, CallRule{Source: "src/cmd/compile/internal/types/size.go", Function: "CalcArraySize", Method: "Typecheck.IR.Types.CalcArraySize"})
	calls = append(calls, CallRule{Source: "src/cmd/compile/internal/types/type.go", Function: "NewArray", Method: "Typecheck.IR.Types.NewArray"})
	calls = append(calls, CallRule{Source: "src/cmd/compile/internal/types/type.go", Function: "NewSlice", Method: "Typecheck.IR.Types.NewSlice"})
	calls = append(calls, CallRule{Source: "src/cmd/compile/internal/types/type.go", Function: "NewTuple", Method: "Typecheck.IR.Types.NewTuple"})
	calls = append(calls, CallRule{Source: "src/cmd/compile/internal/types/type.go", Function: "NewPtr", Method: "Typecheck.IR.Types.NewPtr"})
	calls = append(calls, CallRule{Source: "src/cmd/compile/internal/types/type.go", Function: "NewStruct", Method: "Typecheck.IR.Types.NewStruct"})
	calls = append(calls, CallRule{Source: "src/cmd/compile/internal/types/type.go", Function: "IsDirectIface", Method: "Typecheck.IR.Types.IsDirectIface"})
	calls = append(calls, CallRule{Source: "src/cmd/compile/internal/types/type.go", Function: "FloatForComplex", Method: "Typecheck.IR.Types.FloatForComplex"})
	calls = append(calls, CallRule{Source: "src/cmd/compile/internal/types/identity.go", Function: "Identical", Method: "Typecheck.IR.Types.Identical"})
	calls = append(calls, CallRule{Source: "src/cmd/compile/internal/typecheck/typecheck.go", Function: "Exprs", Method: "Typecheck.Exprs"})
	calls = append(calls, CallRule{Source: "src/cmd/compile/internal/typecheck/typecheck.go", Function: "Stmts", Method: "Typecheck.Stmts"})
	calls = append(calls, CallRule{Source: "src/cmd/compile/internal/typecheck/dcl.go", Function: "TempAt", Method: "Typecheck.TempAt"})
	calls = append(calls, CallRule{Source: "src/cmd/compile/internal/typecheck/syms.go", Function: "LookupRuntimeFunc", Method: "Typecheck.LookupRuntimeFunc"})
	calls = append(calls, CallRule{Source: "src/cmd/compile/internal/typecheck/syms.go", Function: "LookupRuntimeVar", Method: "Typecheck.LookupRuntimeVar"})
	calls = append(calls, CallRule{Source: "src/cmd/compile/internal/typecheck/func.go", Function: "NewClosureStructIter", Method: "Typecheck.NewClosureStructIter"})
	calls = append(calls, CallRule{Source: "src/cmd/compile/internal/reflectdata/map.go", Function: "MapType", Method: "SSA.ReflectData.MapType"})
	calls = append(calls, CallRule{Source: "src/cmd/compile/internal/reflectdata/reflect.go", Function: "TypeLinksym", Method: "SSA.ReflectData.TypeLinksym"})
	calls = append(calls, CallRule{Source: "src/cmd/compile/internal/staticdata/data.go", Function: "FuncLinksym", Method: "SSA.ReflectData.StaticData.FuncLinksym"})
	calls = append(calls, CallRule{Source: "src/cmd/compile/internal/objw/objw.go", Function: "Uvarint", Method: "SSA.ReflectData.Runtime.Writer.Uvarint"})
	calls = append(calls, CallRule{Source: "src/cmd/compile/internal/ssa/config.go", Function: "NewTypes", Method: "SSA.NewTypes"})
	calls = append(calls, CallRule{Source: "src/cmd/compile/internal/ssa/config.go", Function: "NewConfig", Method: "SSA.NewConfig"})
	calls = append(calls, CallRule{Source: "src/cmd/compile/internal/ssa/rewrite.go", Function: "IsInlinableMemmove", Method: "SSA.IsInlinableMemmove"})
	calls = append(calls, CallRule{Source: "src/cmd/compile/internal/ssa/rewrite.go", Function: "LogLargeCopy", Method: "SSA.LogLargeCopy"})
	calls = append(calls, CallRule{Source: root + "base/print.go", Function: "Assert", Method: "assertCondition"})
	calls = append(calls, CallRule{Source: root + "base/print.go", Function: "Warn", Method: "warn"})
	for i := range units {
		u := &units[i]
		if u.Source != root+"ssagen/ssa.go" {
			continue
		}
		u.State = states
		u.Calls = calls
		u.Functions = unique(u.Functions, []string{"DumpInline", "InitEnv", "InitConfig", "InitTables", "AbiForBodylessFuncStackMap", "abiForFunc", "buildssa", "allocSizeAndAlign", "allocSize", "allocAlign", "dumpSourcesColumn", "readFuncLines", "ssaMarker", "peelConvNop", "softfloatInit", "etypesign", "fieldIdx", "clobberBase", "callTargetLSym", "isStructNotSIMD"})
		u.Declarations = unique(u.Declarations, []string{"ssaDumpFile", "maxAggregatedHeapAllocation", "state.emitOpenDeferInfo", "state.storeParameterRegsToStack", "state.zeroResults", "state.paramsToHeap", "state.newHeapaddr", "state.flushPendingHeapAllocations", "state.specializedMallocSym", "state.sizeSpecializedMallocEnabled", "state.setHeapaddr", "state.newObject", "state.newObjectNonSpecialized", "state.checkPtrAlignment", "state.reflectType", "state.updateUnsetPredPos", "openDeferInfo", "state", "backingStoreInfo", "funcLine", "ssaLabel", "state.label", "state.Logf", "state.Log", "state.Fatalf", "state.Warnl", "state.Debug_checknil", "state.startBlock", "state.endBlock", "state.pushLine", "state.popLine", "state.peekPos", "state.newValue0", "state.newValue0A", "state.newValue0I", "state.newValue1", "state.newValue1A", "state.newValue1Apos", "state.newValue1I", "state.newValue2", "state.newValue2A", "state.newValue2Apos", "state.newValue2I", "state.newValue3", "state.newValue3I", "state.newValue3A", "state.newValue3Apos", "state.newValue4", "state.newValue4A", "state.newValue4I", "state.entryBlock", "state.entryNewValue0", "state.entryNewValue0A", "state.entryNewValue1", "state.entryNewValue1I", "state.entryNewValue1A", "state.entryNewValue2", "state.entryNewValue2A", "state.constSlice", "state.constInterface", "state.constNil", "state.constEmptyString", "state.constBool", "state.constInt8", "state.constInt16", "state.constInt32", "state.constInt64", "state.constFloat32", "state.constFloat64", "state.constInt", "state.newValueOrSfCall1", "state.newValueOrSfCall2", "instrumentKind", "instrumentRead", "state.instrument", "state.instrumentFields", "state.instrumentMove", "state.instrument2", "state.load", "state.rawLoad", "state.store", "state.zero", "state.move", "state.moveWhichMayOverlap", "state.stmtList", "state.stmt", "shareDeferExits", "state.exit", "opAndType", "state.concreteEtype", "state.ssaOp", "opAndTwoTypes", "twoTypes", "twoOpsAndType", "state.ssaShiftOp", "state.uintptrConstant", "state.conv", "state.expr", "state.exprCheckPtr", "state.resultOfCall", "state.resultAddrOfCall", "state.getBackingStoreInfoForAppend", "state.getBackingStoreInfo", "state.append", "state.move2heap", "state.minMax", "state.ternary", "state.condBranch", "skipMask", "skipPtr", "state.assign", "state.assignWhichMayOverlap", "state.zeroVal", "callKind", "callNormal", "sfRtCallDef", "state.sfcall", "state.split", "state.intrinsicCall", "state.intrinsicArgs", "state.openDeferRecord", "state.openDeferSave", "state.openDeferExit", "state.callResult", "state.callAddr", "state.call", "state.maybeNilCheckClosure", "state.getClosureAndRcvr", "state.addr", "state.canSSA", "state.canSSAName", "state.exprPtr", "state.nilCheck", "state.boundsCheckArrayIndex", "state.boundsCheck", "state.check", "state.intDivide", "state.rtcall", "state.storeType", "state.storeTypeScalars", "state.storeTypePtrs", "state.putArg", "state.slice", "u642fcvtTab", "state.uint64Tofloat64", "state.uint64Tofloat32", "state.uint64Tofloat", "u322fcvtTab", "state.uint32Tofloat64", "state.uint32Tofloat32", "state.uint32Tofloat", "state.referenceTypeBuiltin", "f2uCvtTab", "state.float32ToUint64", "state.float64ToUint64", "state.float32ToUint32", "state.float64ToUint32", "state.floatToUint", "state.dottype", "state.dynamicDottype", "state.dottype1", "state.temp", "state.variable", "state.mem", "state.addNamedValue", "state.extendIndex"})
		for _, name := range []string{"ssaConfig", "ssaCaches", "ssaDump", "ssaDir", "ssaDumpStdout", "ssaDumpCFG", "ssaDumpInlined", "memVar", "ptrVar", "lenVar", "capVar", "typVar", "okVar", "deferBitsVar", "hashVar", "opToSSA", "fpConvOpToSSA", "fpConvOpToSSA32", "uint64fpConvOpToSSA", "shiftOpToSSA", "softFloatOps", "u64_f64", "u64_f32", "u32_f64", "u32_f32", "f32_u64", "f64_u64", "f32_u32", "f64_u32"} {
			u.Context.Fields[name] = name
		}
		u.Owner.Types = append(u.Owner.Types, "state")
		u.HostCalls = append(u.HostCalls, HostCallRule{Package: "os", Function: "MkdirAll", Method: "makeDumpDirectory"})
		u.Checks["buildssa"] = "checkBuildSSA"
		u.Calls = append(u.Calls, CallRule{Source: root + "ssa/compile.go", Function: "Compile", Method: "compileConstructedSSA"})
		u.FunctionContinuations = map[string]FunctionContinuationRule{"buildssa": {Before: SourceCallAnchor{Package: "cmd/compile/internal/ssa", Member: "Compile"}, Method: "finishSSAConstruction", State: "s"}}
		u.Checks["InitTables"] = "checkIntrinsicTables"
	}
	context := *frame.Context
	context.Fields = nil
	units = append(units, Unit{Source: root + "ssagen/phi.go", WholeFile: true, Context: &context, State: states, Calls: calls, Owner: &OwnerRule{Types: []string{"phiState", "simplePhiState"}, Context: "Context", Field: "compilation"}})
	intrinsicContext := context
	intrinsicContext.Fields = map[string]string{"intrinsics": "intrinsics", "ssaVecBySize": "ssaVecBySize", "cvtVToMaskOpcodes": "cvtVToMaskOpcodes", "cvtMaskToVOpcodes": "cvtMaskToVOpcodes"}
	units = append(units, Unit{Source: root + "ssagen/intrinsics.go", WholeFile: true, Context: &intrinsicContext, State: states, Calls: calls, ContextParams: []string{"intrinsicBuilders.*"}, Checks: map[string]string{"GenIntrinsicBody": "checkIntrinsicBody", "IsIntrinsicSym": "checkIntrinsicSymbol", "IsIntrinsicCall": "checkIntrinsicCall"}})
	for _, file := range []string{"simdAMD64intrinsics.go", "simdARM64intrinsics.go", "simdWasmintrinsics.go"} {
		units = append(units, Unit{Source: root + "ssagen/" + file, WholeFile: true, Context: &context, State: states, Calls: calls})
	}
	return units
}

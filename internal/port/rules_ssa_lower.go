package port

// NewConfig retains all architecture callbacks; these are the native lowering
// files and their common rewrite closure. logRule is for -log regenerated rules,
// absent from every pinned generated caller, and remains outside this closure.
func ssaLoweringMigration(units []Unit) []Unit {
	const root = "src/cmd/compile/internal/"
	context := func() *ContextRule {
		return &ContextRule{Type: "Context", Constructor: "initialize", Initialize: true, Selective: true, RedirectTrace: true, Adapter: "adapter_context.go"}
	}
	states := []StateRule{
		{Source: root + "base/link.go", Variable: "Ctxt", Field: "IR.Base.Ctxt"},
		{Source: root + "base/hashdebug.go", Variable: "ConvertHash", Field: "IR.Base.ConvertHash"},
		{Source: root + "base/hashdebug.go", Variable: "FmaHash", Field: "IR.Base.FmaHash"},
		{Source: root + "types/size.go", Variable: "PtrSize", Field: "ptrSize"},
		{Source: root + "rttype/rttype.go", Variable: "Type", Field: "ReflectData.Runtime.Type"},
		{Source: root + "rttype/rttype.go", Variable: "PtrType", Field: "ReflectData.Runtime.PtrType"},
		{Source: "src/internal/buildcfg/cfg.go", Variable: "GOOS", Field: "IR.Base.Ctxt.TargetOS"},
		{Source: "src/internal/buildcfg/cfg.go", Variable: "GOAMD64", Field: "GOAMD64"},
		{Source: "src/internal/buildcfg/cfg.go", Variable: "GOARM", Field: "GOARM"},
		{Source: "src/internal/buildcfg/cfg.go", Variable: "GOPPC64", Field: "GOPPC64"},
		{Source: "src/internal/buildcfg/cfg.go", Variable: "GORISCV64", Field: "GORISCV64"},
	}
	for _, name := range []string{"Types", "TypeMem", "TypeFlags", "TypeInt128", "TypeVec128", "TypeVec256", "TypeVec512", "TypeMask"} {
		states = append(states, StateRule{Source: root + "types/type.go", Variable: name, Field: "Types." + name})
	}
	calls := []CallRule{{Source: root + "base/print.go", Function: "Fatalf", Method: "fatalf"}}
	for _, group := range []struct {
		file, prefix string
		names        []string
	}{
		{"types/type.go", "Types.", []string{"NewTuple", "IsDirectIface"}},
		{"types/size.go", "Types.", []string{"PtrDataSize"}},
		{"types/alg.go", "Types.", []string{"IsComparable"}},
		{"types/fmt.go", "Types.", []string{"TypeHash"}},
		{"reflectdata/reflect.go", "ReflectData.", []string{"GCSym", "TypeLinksym", "MarkTypeSymUsedInInterface", "ABIKindOfType"}},
		{"rttype/rttype.go", "ReflectData.Runtime.", []string{"FromReflect"}},
		{"logopt/log_opts.go", "Log.", []string{"Enabled", "LogOpt"}},
	} {
		for _, name := range group.names {
			calls = append(calls, CallRule{Source: root + group.file, Function: name, Method: group.prefix + name})
		}
	}
	for i := range units {
		u := &units[i]
		switch u.Source {
		case root + "ssa/regalloc.go":
			u.Declarations = append(u.Declarations, "Value.rematerializeable")
		case root + "ssa/opGen.go":
			u.ConstantFields = append(u.ConstantFields, ConstantFieldRule{Variable: "opcodeTable", Field: "rematerializeable", Functions: []string{"Value.rematerializeable"}}, ConstantFieldRule{Variable: "opcodeTable", Field: "fixedReg", Functions: []string{"Value.rematerializeable"}}, ConstantFieldRule{Variable: "opcodeTable", Field: "generic", Functions: []string{"checkLower"}})
		case root + "types/type.go":
			u.Declarations = append(u.Declarations, "Type.SimpleString", "Type.FieldOff")
		case root + "ssa/config.go":
			u.WholeFile = true
			u.Declarations = nil
			u.Functions = nil
			u.Calls = append(u.Calls, CallRule{Source: root + "abi/abiutils.go", Function: "NewABIConfig", Method: "ABI.NewABIConfig"}, CallRule{Source: root + "base/print.go", Function: "Fatalf", Method: "fatalf"})
			u.ContextFrom = map[string]ContextSource{"Config.haveByteSwap": {Argument: "c", Resolver: "configContext"}}
			u.Checks["NewConfig"] = "checkConfig"
		case root + "ssa/func.go":
			u.Declarations = append(u.Declarations, "Func.useFMA")
			u.ContextFrom["Func.useFMA"] = ContextSource{Argument: "f", Resolver: "diagnosticContext"}
			u.State = append(u.State, StateRule{Source: root + "base/hashdebug.go", Variable: "FmaHash", Field: "IR.Base.FmaHash"})
		case root + "ssa/rewrite.go":
			u.StatelessFunctions = []string{"oneBit", "isPowerOfTwo"}
			u.Functions = []string{"applyRewrite", "is64BitFloat", "is32BitFloat", "is64BitInt", "is32BitInt", "is16BitInt", "is8BitInt", "isPtr", "copyCompatibleType", "mergeSym", "canMergeSym", "canMergeLoadClobber", "canMergeLoad", "isSameCall", "isMalloc", "isNewObject", "isSpecializedMalloc", "canLoadUnaligned", "nlz64", "nlz32", "nlz16", "nlz8", "ntz64", "ntz32", "ntz16", "ntz8", "oneBit", "nto", "log8", "log16", "log32", "log64", "log8u", "log16u", "log32u", "log64u", "isPowerOfTwo", "is32Bit", "is16Bit", "is8Bit", "isU8Bit", "is12Bit", "isU12Bit", "isU16Bit", "isU32Bit", "is20Bit", "b2i", "b2i32", "canMulStrengthReduce", "canMulStrengthReduce32", "mulStrengthReduce", "mulStrengthReduce32", "shiftIsBounded", "canonLessThan", "truncate64Fto32F", "DivisionNeedsFixUp", "auxTo32F", "auxIntToBool", "auxIntToInt8", "auxIntToInt16", "auxIntToInt32", "auxIntToInt64", "auxIntToUint8", "auxIntToUint64", "auxIntToFloat32", "auxIntToFloat64", "auxIntToValAndOff", "auxIntToArm64BitField", "auxIntToArm64ConditionalParams", "auxIntToFlagConstant", "auxIntToOp", "boolToAuxInt", "int8ToAuxInt", "int16ToAuxInt", "int32ToAuxInt", "int64ToAuxInt", "uint8ToAuxInt", "uint64ToAuxInt", "float32ToAuxInt", "float64ToAuxInt", "valAndOffToAuxInt", "arm64BitFieldToAuxInt", "arm64ConditionalParamsToAuxInt", "flagConstantToAuxInt", "opToAuxInt", "auxToString", "auxToSym", "auxToType", "auxToCall", "auxToS390xCCMask", "auxToS390xRotateParams", "StringToAux", "symToAux", "callToAux", "typeToAux", "s390xCCMaskToAux", "s390xRotateParamsToAux", "uaddOvf", "devirtLECall", "isSamePtr", "isStackPtr", "disjoint", "disjointTypes", "moveSize", "mergePoint", "clobber", "resetCopy", "clobberIfDead", "noteRule", "countRule", "warnRule", "flagArg", "amd64CapAVXShift", "arm64Negate", "arm64Invert", "ccARM64Eval", "isConstZero", "reciprocalExact64", "reciprocalExact32", "isARMImmRot", "overlap", "ZeroUpper32Bits", "ZeroUpper48Bits", "ZeroUpper56Bits", "isInlinableMemclr", "isInlinableMemmove", "IsInlinableMemmove", "logLargeCopy", "LogLargeCopy", "hasSmallRotate", "supportsPPC64PCRel", "newPPC64ShiftAuxInt", "GetPPC64Shiftsh", "GetPPC64Shiftmb", "isPPC64WordRotateMask", "isPPC64WordRotateMaskNonWrapping", "encodePPC64RotateMask", "mergePPC64RLDICLandSRDconst", "DecodePPC64RotateMask", "isPPC64ValidShiftMask", "getPPC64ShiftMaskLength", "mergePPC64RShiftMask", "mergePPC64AndSrwi", "mergePPC64AndSrdi", "mergePPC64AndSldi", "mergePPC64ClrlsldiSrw", "mergePPC64ClrlsldiSrd", "mergePPC64ClrlsldiRlwinm", "mergePPC64AndRlwinm", "mergePPC64MovwzregRlwinm", "mergePPC64RlwinmAnd", "mergePPC64SldiRlwinm", "mergePPC64SldiSrw", "convertPPC64OpToOpCC", "convertPPC64RldiclAndccconst", "rotateLeft32", "rotateRight64", "armBFAuxInt", "isARM64BFMask", "arm64BFWidth", "arm64ConditionalParamsAuxInt", "arm64ConditionalParamsAuxIntWithValue", "registerizable", "needRaceCleanup", "symIsRO", "symIsROZero", "isFixedLoad", "rewriteFixedLoad", "read8", "read16", "read32", "read64", "sequentialAddresses", "addFlags64", "subFlags64", "addFlags32", "subFlags32", "logicFlags64", "logicFlags32", "makeJumpTableSym", "canRotate", "isARM64bitcon", "sequenceOfOnes", "isARM64addcon", "setPos", "isNonNegative", "rewriteStructLoad", "rewriteStructStore", "isDirectAndComparableType", "isDirectAndComparableType1", "isDirectAndComparableType2", "isDirectAndComparableIface", "isDirectAndComparableIface1", "isDirectAndComparableIface2", "bitsAdd64", "bitsMulU64", "bitsMulU32", "flagify", "auxToPanicBoundsC", "auxToPanicBoundsCC", "panicBoundsCToAux", "panicBoundsCCToAux", "isDictArgSym", "imakeOfStructMake", "bool2int", "rewriteCondSelectIntoMath", "addToSub", "modularMultiplicativeInverse"}
			u.Declarations = []string{"auxMark.CanBeAnSSAAux", "stringAux.CanBeAnSSAAux", "arm64BitField.lsb", "arm64BitField.width", "arm64ConditionalParams.Cond", "arm64ConditionalParams.Nzcv", "arm64ConditionalParams.ConstValue", "flagConstant.N", "flagConstant.Z", "flagConstant.C", "flagConstant.V", "flagConstant.eq", "flagConstant.ne", "flagConstant.lt", "flagConstant.le", "flagConstant.gt", "flagConstant.ge", "flagConstant.ult", "flagConstant.ule", "flagConstant.ugt", "flagConstant.uge", "flagConstant.ltNoov", "flagConstant.leNoov", "flagConstant.gtNoov", "flagConstant.geNoov", "flagConstant.String", "flagConstantBuilder.encode", "PanicBoundsC.CanBeAnSSAAux", "PanicBoundsCC.CanBeAnSSAAux", "deadValueChoice", "Aux", "auxMark", "stringAux", "flagConstant", "flagConstantBuilder", "PanicBoundsC", "PanicBoundsCC", "leaveDeadValues"}
			u.Context = context()
			u.Context.Fields = map[string]string{"AuxMark": "AuxMark"}
			u.State = states
			u.Calls = calls
			u.ContextFrom = map[string]ContextSource{"applyRewrite": {Argument: "f", Resolver: "diagnosticContext"}}
		}
	}
	for _, file := range []string{"rewrite386.go", "rewrite386splitload.go", "rewriteAMD64.go", "rewriteAMD64latelower.go", "rewriteAMD64splitload.go", "rewriteARM.go", "rewriteARM64.go", "rewriteARM64latelower.go", "rewriteLOONG64.go", "rewriteLOONG64latelower.go", "rewriteMIPS.go", "rewriteMIPS64.go", "rewriteMIPS64latelower.go", "rewritePPC64.go", "rewritePPC64latelower.go", "rewriteRISCV64.go", "rewriteRISCV64latelower.go", "rewriteS390X.go", "rewriteWasm.go"} {
		units = append(units, Unit{Source: root + "ssa/" + file, WholeFile: true, Context: context(), State: states, Calls: calls})
	}
	units = append(units, Unit{Source: root + "ssa/magic.go", WholeFile: true}, Unit{Source: root + "ssa/lower.go", WholeFile: true}, Unit{Source: "src/internal/buildcfg/cfg.go", Declarations: []string{"GoarmFeatures"}})
	return units
}

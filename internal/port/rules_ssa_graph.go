package port

// Preserve the full native graph layout, including backend analysis and debug state.
func ssaGraphMigration() []Unit {
	return []Unit{
		{Source: "src/cmd/compile/internal/ssa/copyelim.go", WholeFile: true},
		{Source: "src/cmd/compile/internal/ssa/dom.go", Functions: []string{"postorder", "postorderWithNumbering", "dominators", "compressOrig", "evalOrig", "linkOrig"}, Declarations: []string{"blockAndIndex", "Func.dominatorsLTOrig", "Func.dfsOrig"}},
		{Source: "src/cmd/compile/internal/ssa/known_bits.go", Declarations: []string{"knownBitsEntry"}},
		{Source: "src/cmd/compile/internal/ssa/sparsemappos.go", WholeFile: true},
		{Source: "src/cmd/compile/internal/ssa/allocators.go", WholeFile: true, SliceReinterpretations: nativeAllocatorReinterpretations(), ReceiverState: &ReceiverStateRule{Type: "Cache", Fields: map[string]string{
			"poolFreeValueSlice": "poolFreeValueSlice", "poolFreeLimitSlice": "poolFreeLimitSlice", "poolFreeSparseSet": "poolFreeSparseSet", "poolFreeSparseMap": "poolFreeSparseMap", "poolFreeSparseMapPos": "poolFreeSparseMapPos",
		}}},
		{Source: "src/cmd/compile/internal/ssa/numberlines.go", WholeFile: true, Context: &ContextRule{Type: "Context", Constructor: "initialize", Initialize: true, Selective: true, RedirectTrace: true, Adapter: "adapter_context.go"}, ContextFrom: map[string]ContextSource{"numberLines": {Argument: "f", Resolver: "diagnosticContext"}}},
		{Source: "src/cmd/compile/internal/ssa/biasedsparsemap.go", WholeFile: true},
		{Source: "src/cmd/compile/internal/ssa/cache.go", WholeFile: true},
		{Source: "src/cmd/compile/internal/ssa/compile.go", Declarations: []string{"pass"}},
		{Source: "src/cmd/compile/internal/ssa/cse.go", Declarations: []string{"auxmap"}},
		{Source: "src/cmd/compile/internal/ssa/debug.go", Declarations: []string{"BlockDebug", "LocListEntry", "RegisterSet", "SlotID", "StackOffset", "VarID", "VarLoc", "debugState", "pendingEntry", "stateAtPC"}},
		{Source: "src/cmd/compile/internal/ssa/func.go", Functions: []string{"FuncNameABI"}, Declarations: []string{"Func", "LocalSlotSplitKey", "constSliceMagic", "Func.unCacheLine", "Func.unCache", "Func.freeValue", "Func.freeBlock", "Func.constVal", "Func.ConstBool", "Func.ConstInt8", "Func.ConstInt16", "Func.ConstInt32", "Func.ConstInt64", "Func.ConstFloat32", "Func.ConstFloat64", "Func.ConstSlice", "Func.ConstInterface", "Func.ConstNil", "Func.ConstEmptyString", "Func.ConstOffPtrSP", "Func.LogStat", "Func.spSb", "Func.postorder", "Func.Postorder", "Func.Idom", "Func.Sdom", "Func.newSparseSet", "Func.retSparseSet", "Func.newSparseMap", "Func.retSparseMap", "Func.newSparseMapPos", "Func.retSparseMapPos", "Config.NewFunc", "Func.NumBlocks", "Func.NumValues", "Func.newValue", "Func.newValueNoBlock", "Func.NewBlock", "Func.invalidateCFG", "Func.Frontend", "Func.Warnl", "Func.Logf", "Func.Log", "Func.NameABI", "Block.NewValue0", "Block.NewValue0I", "Block.NewValue0A", "Block.NewValue0IA", "Block.NewValue1", "Block.NewValue1I", "Block.NewValue1A", "Block.NewValue1IA", "Block.NewValue2", "Block.NewValue2A", "Block.NewValue2I", "Block.NewValue2IA", "Block.NewValue3", "Block.NewValue3I", "Block.NewValue3A", "Block.NewValue4", "Block.NewValue4A", "Block.NewValue4I"}},
		{Source: "src/cmd/compile/internal/ssa/html.go", Declarations: []string{"HTMLWriter", "dotWriter"}},
		{Source: "src/cmd/compile/internal/ssa/likelyadjust.go", Declarations: []string{"loop", "loopnest"}},
		{Source: "src/cmd/compile/internal/ssa/poset.go", Declarations: []string{"bitset", "poset", "posetEdge", "posetNode", "posetUndo", "undoType"}},
		{Source: "src/cmd/compile/internal/ssa/prove.go", Declarations: []string{"limit"}},
		ssaRewriteAuxUnit(),
		{Source: "src/cmd/compile/internal/ssa/check.go", Functions: []string{"isExactFloat32"}},
		{Source: "src/cmd/compile/internal/ssa/sparsemap.go", WholeFile: true},
		{Source: "src/cmd/compile/internal/ssa/sparseset.go", WholeFile: true},
		{Source: "src/cmd/compile/internal/ssa/sparsetree.go", WholeFile: true},
		{Source: "src/cmd/compile/internal/ssa/stackalloc.go", Declarations: []string{"stackAllocState", "stackUseBlock", "stackValState"}},
		{Source: "src/cmd/compile/internal/ssa/xposmap.go", WholeFile: true},
	}
}

func nativeAllocatorReinterpretations() []SliceReinterpretationRule {
	var rules []SliceReinterpretationRule
	for _, name := range []string{"BlockSlice", "Int64", "IntSlice", "Int32Slice", "Int8Slice", "BoolSlice", "IDSlice", "UintSlice", "KnownBitsEntriesSlice"} {
		base := "LimitSlice"
		if name == "BlockSlice" {
			base = "ValueSlice"
		}
		rules = append(rules, SliceReinterpretationRule{Allocate: "Cache.alloc" + name, Free: "Cache.free" + name, BaseAllocate: "Cache.alloc" + base, BaseFree: "Cache.free" + base})
	}
	return rules
}

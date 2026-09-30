package port

// Preserve the full native graph layout, including backend analysis and debug state.
func ssaGraphMigration() []Unit {
	return []Unit{
		{Source: "src/cmd/compile/internal/ssa/numberlines.go", Functions: []string{"notStmtBoundary"}, Declarations: []string{"Block.FirstPossibleStmtValue"}},
		{Source: "src/cmd/compile/internal/ssa/biasedsparsemap.go", Declarations: []string{"biasedSparseMap"}},
		{Source: "src/cmd/compile/internal/ssa/cache.go", WholeFile: true},
		{Source: "src/cmd/compile/internal/ssa/compile.go", Declarations: []string{"pass"}},
		{Source: "src/cmd/compile/internal/ssa/cse.go", Declarations: []string{"auxmap"}},
		{Source: "src/cmd/compile/internal/ssa/debug.go", Declarations: []string{"BlockDebug", "LocListEntry", "RegisterSet", "SlotID", "StackOffset", "VarID", "VarLoc", "debugState", "pendingEntry", "stateAtPC"}},
		{Source: "src/cmd/compile/internal/ssa/func.go", Functions: []string{"FuncNameABI"}, Declarations: []string{"Func", "LocalSlotSplitKey", "Config.NewFunc", "Func.NumBlocks", "Func.NumValues", "Func.newValue", "Func.newValueNoBlock", "Func.NewBlock", "Func.invalidateCFG", "Func.Frontend", "Func.Warnl", "Func.Logf", "Func.Log", "Func.NameABI", "Block.NewValue0", "Block.NewValue0I", "Block.NewValue0A", "Block.NewValue0IA", "Block.NewValue1", "Block.NewValue1I", "Block.NewValue1A", "Block.NewValue1IA", "Block.NewValue2", "Block.NewValue2A", "Block.NewValue2I", "Block.NewValue2IA", "Block.NewValue3", "Block.NewValue3I", "Block.NewValue3A", "Block.NewValue4", "Block.NewValue4A", "Block.NewValue4I"}},
		{Source: "src/cmd/compile/internal/ssa/html.go", Declarations: []string{"HTMLWriter", "dotWriter"}},
		{Source: "src/cmd/compile/internal/ssa/likelyadjust.go", Declarations: []string{"loop", "loopnest"}},
		{Source: "src/cmd/compile/internal/ssa/poset.go", Declarations: []string{"bitset", "poset", "posetEdge", "posetNode", "posetUndo", "undoType"}},
		{Source: "src/cmd/compile/internal/ssa/prove.go", Declarations: []string{"limit"}},
		{Source: "src/cmd/compile/internal/ssa/rewrite.go", Declarations: []string{"Aux"}},
		{Source: "src/cmd/compile/internal/ssa/sparsemap.go", Declarations: []string{"genericSparseMap", "sparseEntry", "sparseKey", "sparseMap"}},
		{Source: "src/cmd/compile/internal/ssa/sparseset.go", Declarations: []string{"sparseSet"}},
		{Source: "src/cmd/compile/internal/ssa/sparsetree.go", Declarations: []string{"SparseTree", "SparseTreeNode"}},
		{Source: "src/cmd/compile/internal/ssa/stackalloc.go", Declarations: []string{"stackAllocState", "stackUseBlock", "stackValState"}},
		{Source: "src/cmd/compile/internal/ssa/xposmap.go", Declarations: []string{"xposmap"}},
	}
}

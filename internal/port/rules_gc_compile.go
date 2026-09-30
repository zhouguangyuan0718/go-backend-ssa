package port

// gcCompileMigration preserves the native function preparation and worker queue.
func gcCompileMigration(units []Unit) []Unit {
	const root = "src/cmd/compile/internal/"
	var gc Unit
	for i := range units {
		u := &units[i]
		switch u.Source {
		case "src/cmd/internal/obj/link.go":
			u.Declarations = append(u.Declarations, "WasmI32")
		case root + "gc/obj.go":
			gc = *u
		case root + "ssagen/pgen.go":
			u.WholeFile = true
			u.Functions = nil
			u.Declarations = nil
			u.Context.Fields = map[string]string{"globalMapInitLsyms": "globalMapInitLsyms", "largeStackFramesMu": "largeStackFramesMu", "largeStackFrames": "largeStackFrames"}
			u.Checks["Compile"] = "checkCompile"
			u.Calls = append(u.Calls, CallRule{Source: root + "inline/inl.go", Function: "IsPgoHotFunc", Method: "Inline.IsPgoHotFunc"}, CallRule{Source: root + "inline/inl.go", Function: "HasPgoHotInline", Method: "Inline.HasPgoHotInline"}, CallRule{Source: root + "objw/prog.go", Function: "NewProgs", Method: "SSA.ReflectData.Runtime.Writer.NewProgs"})
		case root + "ssagen/abi.go":
			u.WholeFile = true
			u.Functions = nil
			u.Declarations = nil
			u.State = append(u.State, StateRule{Source: "src/internal/buildcfg/cfg.go", Variable: "GOARCH", Field: "Typecheck.IR.Base.Ctxt.Arch.Name"}, StateRule{Source: root + "types/size.go", Variable: "RegSize", Field: "regSize"}, StateRule{Source: root + "types/size.go", Variable: "PtrSize", Field: "ptrSize"})
			u.Calls = append(u.Calls, CallRule{Source: root + "base/print.go", Function: "FatalfAt", Method: "fatalfAt"}, CallRule{Source: root + "ir/abi.go", Function: "InitLSym", Method: "Typecheck.IR.InitLSym"}, CallRule{Source: root + "types/size.go", Function: "RoundUp", Method: "Typecheck.IR.Types.RoundUp"}, CallRule{Source: root + "objw/prog.go", Function: "NewProgs", Method: "SSA.ReflectData.Runtime.Writer.NewProgs"})
		}
	}
	ctx := *gc.Context
	ctx.Fields = map[string]string{"compilequeue": "compilequeue"}
	state := append([]StateRule{}, gc.State...)
	state = append(state, StateRule{Source: root + "ir/expr.go", Variable: "IsIntrinsicSym", Field: "Package.Noder.Typecheck.IR.IsIntrinsicSym"}, StateRule{Source: root + "base/debug.go", Variable: "Debug", Field: "Package.Noder.Typecheck.IR.Debug"}, StateRule{Source: root + "ir/func.go", Variable: "CurFunc", Field: "Package.Noder.Typecheck.IR.CurFunc"}, StateRule{Source: root + "staticinit/sched.go", Variable: "MapInitToVar", Field: "Package.Noder.StaticInit.MapInitToVar"}, StateRule{Source: root + "types/size.go", Variable: "CalcSizeDisabled", Field: "Writer.Types.CalcSizeDisabled"}, StateRule{Source: "src/internal/race/norace.go", Variable: "Enabled", Field: "RaceEnabled"})
	calls := append([]CallRule{}, gc.Calls...)
	for _, x := range []struct{ file, name, method string }{
		{"base/print.go", "FatalfAt", "fatalfAt"}, {"base/print.go", "Errors", "errors"},
		{"ir/abi.go", "InitLSym", "Package.Noder.Typecheck.IR.InitLSym"},

		{"ir/dump.go", "AstDump", "Package.Noder.Typecheck.IR.AstDump"},
		{"ir/dump.go", "MatchAstDump", "Package.Noder.Typecheck.IR.MatchAstDump"},
		{"ssagen/abi.go", "CreateWasmImportWrapper", "Backend.CreateWasmImportWrapper"},
		{"ssagen/abi.go", "GenWasmExportWrapper", "Backend.GenWasmExportWrapper"},
		{"ssagen/intrinsics.go", "GenIntrinsicBody", "Backend.GenIntrinsicBody"},
		{"ssagen/ssa.go", "AbiForBodylessFuncStackMap", "Backend.AbiForBodylessFuncStackMap"},
		{"ssagen/ssa.go", "EmitArgInfo", "Backend.EmitArgInfo"},
		{"ssagen/pgen.go", "RegisterMapInitLsym", "Backend.RegisterMapInitLsym"},
		{"ssagen/pgen.go", "Compile", "compileWorker"},
		{"liveness/plive.go", "WriteFuncMap", "Backend.Liveness.WriteFuncMap"},
		{"walk/walk.go", "Walk", "Lowering.Walk"},
	} {
		calls = append(calls, CallRule{Source: root + x.file, Function: x.name, Method: x.method})
	}
	units = append(units, Unit{Source: root + "gc/compile.go", WholeFile: true, Context: &ctx, State: state, Calls: calls, HostCalls: []HostCallRule{{Package: "math/rand", Function: "Perm", Method: "permutation"}}})
	units = append(units, Unit{Source: "src/cmd/internal/obj/wasm/wasmobj.go", Declarations: []string{"GojsModule"}})
	return units
}

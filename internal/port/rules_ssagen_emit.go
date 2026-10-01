package port

// Preserve native SSA instruction emission and its metadata dependencies.
func ssagenEmitMigration(units []Unit) []Unit {
	const root = "src/cmd/compile/internal/"
	var live, debug, backend Unit
	for _, u := range units {
		switch u.Source {
		case root + "ssagen/ssa.go":
			backend = u
		case root + "liveness/plive.go":
			live = u
		case root + "ssa/rewrite.go":
			debug = u
		}
	}
	for i := range units {
		u := &units[i]
		switch u.Source {
		case "src/internal/abi/type.go":
			u.Declarations = append(u.Declarations, "TraceArgsLimit", "TraceArgsEndSeq")
		case root + "abt/avlint32.go":
			u.WholeFile = true
			u.Declarations = nil
		case root + "ssa/op.go":
			u.Declarations = append(u.Declarations, "BoundsKind.Code")
			u.ContextFrom["BoundsKind.Code"] = ContextSource{Argument: "b", Resolver: "boundsKindContext"}
		case root + "ssa/func.go":
			u.Declarations = append(u.Declarations, "Func.localSlotAddr")
		case "src/cmd/internal/dwarf/dwarf.go":
			u.Functions = append(u.Functions, "AppendUleb128", "AppendSleb128")
		case root + "ssagen/ssa.go":
			u.WholeFile = true
			u.Functions = nil
			u.Declarations = nil
			u.Calls = append(u.Calls, CallRule{Source: root + "objw/objw.go", Function: "Uint8", Method: "SSA.ReflectData.Runtime.Writer.Uint8"}, CallRule{Source: root + "objw/objw.go", Function: "SymPtrOff", Method: "SSA.ReflectData.Runtime.Writer.SymPtrOff"})
			u.Owner.Types = append(u.Owner.Types, "State")
			u.Checks["genssa"] = "checkGenSSA"
			u.Literals = append(u.Literals, LiteralRule{Source: root + "ssa/debug.go", Type: "FuncDebug", Method: "SSA.BindFuncDebug"})
			u.State = append(u.State, StateRule{Source: root + "ssa/debug.go", Variable: "BlockStart", Field: "SSA.BlockStart"}, StateRule{Source: root + "ssa/debug.go", Variable: "BlockEnd", Field: "SSA.BlockEnd"}, StateRule{Source: root + "ssa/debug.go", Variable: "FuncEnd", Field: "SSA.FuncEnd"}, StateRule{Source: root + "ssa/compile.go", Variable: "GenssaDump", Field: "SSA.GenssaDump"})
			u.Calls = append(u.Calls, CallRule{Source: root + "liveness/plive.go", Function: "Compute", Method: "Liveness.Compute"}, CallRule{Source: root + "liveness/arg.go", Function: "ArgLiveness", Method: "Liveness.ArgLiveness"})
		case root + "liveness/plive.go":
			u.Checks = map[string]string{"Compute": "checkCompute", "WriteFuncMap": "checkFuncMap"}
			u.WholeFile = true
			u.Functions = nil
			u.Declarations = nil
		case root + "ssa/debug.go":
			u.WholeFile = true
			u.Functions = nil
			u.Declarations = nil
			ctx := *debug.Context
			ctx.Fields = map[string]string{"BlockStart": "BlockStart", "BlockEnd": "BlockEnd", "FuncEnd": "FuncEnd"}
			u.Context = &ctx
			u.State = debug.State
			u.Calls = debug.Calls
			u.ContextParams = []string{"debugState.*"}
			u.Literals = append(u.Literals, LiteralRule{Source: root + "abi/abiutils.go", Type: "ABIParamAssignment", Method: "ABI.BindParamAssignment"})
			u.Checks = map[string]string{"BuildFuncDebug": "checkFuncDebug", "BuildFuncDebugNoOptimized": "checkFuncDebugNoOptimized"}
			u.Owner = &OwnerRule{Types: []string{"FuncDebug"}, Context: "Context", Field: "compilation"}
			u.ContextFrom = map[string]ContextSource{"BuildFuncDebug": {Argument: "f", Resolver: "diagnosticContext"}, "BuildFuncDebugNoOptimized": {Argument: "f", Resolver: "diagnosticContext"}, "PopulateABIInRegArgOps": {Argument: "f", Resolver: "diagnosticContext"}}
		case root + "objw/prog.go":
			u.Functions = []string{"NewProgs", "LosesStmtMark"}
			u.Declarations = append(u.Declarations, "Progs.NewProg", "Progs.Free", "Progs.Prog", "Progs.Clear", "Progs.Append", "Progs.SetText")
			u.Context = &ContextRule{Type: "Context", Constructor: "newContext", Adapter: "adapter_context.go", Selective: true, Fields: map[string]string{"sharedProgArray": "sharedProgArray"}}
			u.Owner = &OwnerRule{Types: []string{"Progs"}, Context: "Context", Field: "compilation"}
			u.State = []StateRule{{Source: root + "base/link.go", Variable: "Ctxt", Field: "Link"}, {Source: root + "base/flag.go", Variable: "Flag", Field: "IR.Base.Flag"}}
			u.Calls = []CallRule{{Source: root + "base/print.go", Function: "Fatalf", Method: "fatalf"}, {Source: root + "base/print.go", Function: "Warn", Method: "warn"}}
			u.Checks = map[string]string{"NewProgs": "checkNewProgs"}
		case "src/cmd/internal/obj/sym.go":
			u.Declarations = append(u.Declarations, "Link.GCLocalsSym")
		case "src/cmd/internal/obj/link.go":
			u.Declarations = append(u.Declarations, "FuncInfo.AddInlMark", "LSym.TextAttrString", "Attribute.String", "TYPE_NONE", "NAME_NONE", "Source", "Addr.Target", "Addr.SetTarget", "Addr.SetConst", "Prog.AddRestSource", "Prog.AddRestSourceReg", "FuncInfo.AddSpill")
			u.Context.Fields = map[string]string{"textAttrStrings": "textAttrStrings"}
			u.ContextFrom = map[string]ContextSource{"Attribute.String": {Argument: "a", Resolver: "attributeLink"}}
		case "src/cmd/internal/objabi/head.go":
			u.Declarations = append(u.Declarations, "Hunknown", "HeadType.Set", "HeadType.String")
		case "src/cmd/internal/obj/util.go":
			u.WholeFile = true
			u.Functions = nil
			u.Declarations = nil
			u.Context = objectDataContext()
			u.Context.Fields = map[string]string{"opSuffixSpace": "opSuffixSpace", "regSpace": "regSpace", "regListSpace": "regListSpace", "spcSpace": "spcSpace", "aSpace": "aSpace"}
			u.ReadTables = []string{"Anames", "armCondCode"}
			u.State = []StateRule{{Source: "src/internal/buildcfg/cfg.go", Variable: "GOARCH", Field: "Arch.Name"}}
			u.ContextParams = []string{"Addr.*"}
			u.ContextParamExports = []string{"As.String", "Addr.WriteNameTo"}
			u.ContextFrom = map[string]ContextSource{}
			for _, n := range []string{"String", "InnermostString", "InstructionString", "WriteInstructionString"} {
				u.ContextFrom["Prog."+n] = ContextSource{Argument: "p", Resolver: "progLink"}
			}
		}
	}
	units = append(units, Unit{Source: root + "liveness/arg.go", WholeFile: true, Checks: map[string]string{"ArgLiveness": "checkArgLiveness"}, Context: live.Context, State: live.State, Calls: live.Calls, Owner: &OwnerRule{Types: []string{"argLiveness"}, Context: "Context", Field: "compilation"}})
	for i := range units {
		u := &units[i]
		if u.Source != root+"liveness/plive.go" && u.Source != root+"liveness/arg.go" {
			continue
		}
		u.State = append(u.State, StateRule{Source: root + "types/type.go", Variable: "TypeVoid", Field: "SSA.Types.TypeVoid"})
		u.Calls = append(u.Calls,
			CallRule{Source: "src/cmd/compile/internal/types/size.go", Function: "PtrDataSize", Method: "SSA.Types.PtrDataSize"},
			CallRule{Source: "src/cmd/compile/internal/reflectdata/reflect.go", Function: "GCSym", Method: "SSA.ReflectData.GCSym"},
			CallRule{Source: "src/cmd/compile/internal/objw/objw.go", Function: "Uint8", Method: "SSA.ReflectData.Runtime.Writer.Uint8"},
			CallRule{Source: "src/cmd/compile/internal/objw/objw.go", Function: "Uint32", Method: "SSA.ReflectData.Runtime.Writer.Uint32"},
			CallRule{Source: "src/cmd/compile/internal/objw/objw.go", Function: "Uintptr", Method: "SSA.ReflectData.Runtime.Writer.Uintptr"},
			CallRule{Source: "src/cmd/compile/internal/objw/objw.go", Function: "SymPtrOff", Method: "SSA.ReflectData.Runtime.Writer.SymPtrOff"},
			CallRule{Source: "src/cmd/compile/internal/objw/objw.go", Function: "Global", Method: "SSA.ReflectData.Runtime.Writer.Global"},
			CallRule{Source: "src/cmd/compile/internal/objw/objw.go", Function: "BitVec", Method: "SSA.ReflectData.Runtime.Writer.BitVec"},
			CallRule{Source: root + "base/print.go", Function: "WarnfAt", Method: "warnfAt"}, CallRule{Source: root + "base/print.go", Function: "ErrorfAt", Method: "errorfAt"}, CallRule{Source: root + "base/print.go", Function: "Assert", Method: "assertCondition"}, CallRule{Source: root + "base/print.go", Function: "FmtPos", Method: "SSA.IR.Base.FmtPos"})
	}
	units = append(units, Unit{Source: "src/cmd/internal/obj/go.go", Functions: []string{"Nopout"}})
	units = append(units, Unit{Source: "src/cmd/internal/dwarf/dwarf_defs.go", Declarations: []string{"DW_OP_addr", "DW_LLE_end_of_list"}})
	ctx := *backend.Context
	ctx.Fields = map[string]string{"nowritebarrierrecCheck": "nowritebarrierrecCheck"}
	units = append(units, Unit{Source: root + "ssagen/nowb.go", WholeFile: true, Context: &ctx, State: backend.State, Calls: append(append([]CallRule{}, backend.Calls...), CallRule{Source: root + "base/print.go", Function: "FmtPos", Method: "Typecheck.IR.Base.FmtPos"}, CallRule{Source: root + "types/type.go", Function: "RuntimeSymName", Method: "Typecheck.IR.Types.RuntimeSymName"}), Owner: &OwnerRule{Types: []string{"nowritebarrierrecChecker"}, Context: "Context", Field: "compilation"}})
	units = append(units, Unit{Source: "src/internal/abi/bounds.go", WholeFile: true})
	return units
}

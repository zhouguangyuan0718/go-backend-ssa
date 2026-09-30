package port

// Preserve the native frontend services and target/runtime initialization used
// by buildssa. ArchInfo and its typed callback declarations remain source types.
func ssagenFrontendMigration(units []Unit) []Unit {
	const root = "src/cmd/compile/internal/"
	var context *ContextRule
	var states []StateRule
	var calls []CallRule
	var existing []Unit
	for _, u := range units {
		if u.Source == root+"ssagen/abi.go" {
			copy := *u.Context
			context = &copy
			states = append(states, u.State...)
			calls = append(calls, u.Calls...)
		}
	}
	for i := range units {
		if units[i].Source == root+"ssa/op.go" {
			units[i].Declarations = append(units[i].Declarations, "BoundsKind", "BoundsIndex")
		}
	}
	var retained []Unit
	for _, unit := range units {
		if unit.sourcePackage() == "cmd/compile/internal/ssagen" {
			existing = append(existing, unit)
		} else {
			retained = append(retained, unit)
		}
	}
	units = retained
	context.Fields = map[string]string{"ssaConfig": "ssaConfig", "ssaCaches": "ssaCaches", "BoundsCheckFunc": "BoundsCheckFunc", "softFloatOps": "softFloatOps", "deferType": "deferType"}
	states = append(states, StateRule{Source: root + "base/flag.go", Variable: "Flag", Field: "Typecheck.IR.Base.Flag"}, StateRule{Source: root + "types/type.go", Variable: "Types", Field: "Typecheck.IR.Types.Types"}, StateRule{Source: root + "types/type.go", Variable: "ByteType", Field: "Typecheck.IR.Types.ByteType"}, StateRule{Source: root + "types/type.go", Variable: "ErrorType", Field: "Typecheck.IR.Types.ErrorType"}, StateRule{Source: root + "types/type.go", Variable: "NewPtrCacheEnabled", Field: "Typecheck.IR.Types.NewPtrCacheEnabled"}, StateRule{Source: root + "types/fmt.go", Variable: "LocalPkg", Field: "Typecheck.IR.Types.LocalPkg"}, StateRule{Source: root + "ir/symtab.go", Variable: "Syms", Field: "Typecheck.IR.Syms"}, StateRule{Source: root + "ir/symtab.go", Variable: "Pkgs", Field: "Typecheck.IR.Pkgs"})
	for _, v := range []struct{ file, name, method string }{
		{"ssa/config.go", "NewTypes", "SSA.NewTypes"}, {"ssa/config.go", "NewConfig", "SSA.NewConfig"},
		{"typecheck/syms.go", "LookupRuntimeFunc", "Typecheck.LookupRuntimeFunc"}, {"typecheck/syms.go", "LookupRuntimeVar", "Typecheck.LookupRuntimeVar"},
		{"staticdata/data.go", "StringSym", "SSA.ReflectData.StaticData.StringSym"},
		{"reflectdata/map.go", "MapType", "SSA.ReflectData.MapType"},
		{"types/type.go", "NewPtr", "Typecheck.IR.Types.NewPtr"}, {"types/type.go", "NewSlice", "Typecheck.IR.Types.NewSlice"}, {"types/type.go", "NewField", "Typecheck.IR.Types.NewField"}, {"types/type.go", "NewNamed", "Typecheck.IR.Types.NewNamed"}, {"types/type.go", "NewStruct", "Typecheck.IR.Types.NewStruct"},
		{"types/size.go", "CalcSize", "Typecheck.IR.Types.CalcSize"}, {"types/size.go", "CalcStructSize", "Typecheck.IR.Types.CalcStructSize"},
		{"ir/name.go", "NewDeclNameAt", "Typecheck.IR.NewDeclNameAt"}, {"base/print.go", "WarnfAt", "warnfAt"},
	} {
		calls = append(calls, CallRule{Source: root + v.file, Function: v.name, Method: v.method})
	}
	u := Unit{Source: root + "ssagen/ssa.go", Context: context, State: states, Calls: calls, Owner: &OwnerRule{Types: []string{"ssafn"}, Context: "Context", Field: "compilation"}, Functions: []string{"InitConfig", "softfloatInit", "deferstruct", "abiForFunc", "AbiForBodylessFuncStackMap"}, Declarations: []string{"ssafn", "ssafn.StringData", "ssafn.SplitSlot", "ssafn.Logf", "ssafn.Log", "ssafn.Fatalf", "ssafn.Warnl", "ssafn.Debug_checknil", "ssafn.UseWriteBarrier", "ssafn.Syslook", "ssafn.Func", "sfRtCallDef", "deferStructFnField", "State", "Branch"}, Checks: map[string]string{"InitConfig": "checkInitConfig", "ssafn.SplitSlot": "checkSplitSlot", "ssafn.StringData": "checkStringData"}, Literals: []LiteralRule{{Source: root + "types/pkg.go", Type: "Pkg", Method: "Typecheck.IR.Types.BindPackage", NilPointer: true}}}
	archContext := *context
	archContext.Fields = map[string]string{"Arch": "Arch"}
	units = append(units, Unit{Source: root + "objw/prog.go", Declarations: []string{"Progs", "StackMapIndex", "StackMapDontCare", "StackMapIndex.StackMapValid"}}, Unit{Source: root + "liveness/plive.go", Declarations: []string{"Map"}})
	units = append(units, existing...)
	units = append(units, u, Unit{Source: root + "ssagen/arch.go", WholeFile: true, Context: &archContext})
	return units
}

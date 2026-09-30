package port

// Preserve the native function flush and DWARF/PC metadata pipeline.
func objectFlushMigration(units []Unit) []Unit {
	const root = "src/cmd/internal/"
	emitter := func(fields map[string]string) *ContextRule {
		return &ContextRule{Type: "Emitter", Constructor: "initialize", Initialize: true, Selective: true, RedirectTrace: true, Adapter: "adapter_context.go", Fields: fields}
	}
	for i := range units {
		u := &units[i]
		switch u.Source {
		case root + "dwarf/dwarf.go":
			u.Functions = []string{"EnableLogging", "MergeRanges", "AppendUleb128", "AppendSleb128", "sevenBitU", "sevenBitS", "Uleb128put", "Sleb128put", "expandPseudoForm", "Abbrevs", "GetAbbrev", "putattr", "PutAttrs", "HasChildren", "PutIntConst", "PutGlobal", "PutBasedRanges", "PutRngListRanges", "isEmptyInlinedCall", "inlChildren", "inlinedVarTable", "putPrunedScopes", "PutAbstractFunc", "dwarfFileIndex", "putInlinedFunc", "emitHiLoPc", "PutConcreteFunc", "PutDefaultFunc", "putparamtypes", "putscope", "concreteVar", "putAbstractVar", "putvar", "byChildIndexCmp"}
			u.Declarations = []string{"InfoPrefix", "ConstInfoPrefix", "CUInfoPrefix", "AbstractFuncSuffix", "Sym", "Var", "Scope", "Range", "FnState", "Scope.UnifyRanges", "Scope.AppendRange", "InlCalls", "InlCall", "Context", "dwAttrForm", "DW_AT_go_kind", "DW_ABRV_NULL", "dwAbbrev", "DWAttr", "DWDie", "FnState.PutRanges"}
			u.Context = emitter(map[string]string{"logDwarf": "logDwarf", "sevenbits": "sevenbits", "abbrevsFinalized": "abbrevsFinalized", "abbrevs": "abbrevs"})
			u.ContextParamExports = []string{"FnState.PutRanges"}
			u.State = []StateRule{{Source: "src/internal/buildcfg/cfg.go", Variable: "GOOS", Field: "goos"}, {Source: "src/internal/buildcfg/exp.go", Variable: "Experiment", Field: "experiment"}}
		case root + "dwarf/dwarf_defs.go":
			u.WholeFile = true
			u.Declarations = nil
		case root + "obj/plist.go":
			u.WholeFile = true
			u.Declarations = nil
			u.Context = objectDataContext()
			u.ContextFrom = map[string]ContextSource{"Flushplist": {Argument: "ctxt", Resolver: "dataLink"}}
			u.Checks = map[string]string{"Flushplist": "checkFlushplist"}
		case root + "obj/inl.go", root + "obj/line.go":
			u.WholeFile = true
			u.Declarations = nil
		case root + "obj/dwarf.go":
			u.WholeFile = true
			u.Declarations = nil
			u.Context = objectDataContext()
			u.ContextFrom = map[string]ContextSource{"putpclcdelta": {Argument: "linkctxt", Resolver: "dataLink"}}
			for _, n := range []string{"Uleb128put", "Sleb128put", "PutAbstractFunc", "PutConcreteFunc", "PutDefaultFunc", "PutIntConst", "PutGlobal"} {
				u.Calls = append(u.Calls, CallRule{Source: root + "dwarf/dwarf.go", Function: n, Method: "DWARF." + n})
			}
		case "src/cmd/compile/internal/objw/prog.go":
			u.Declarations = append(u.Declarations, "Progs.Flush")
			u.ContextFrom = map[string]ContextSource{"Progs.Flush": {Argument: "pp", Resolver: "flushContext"}}
		case root + "obj/link.go":
			u.Declarations = append(u.Declarations, "FuncInfo.RecordAutoType", "Attribute.Wrapper", "LSym.NewVarInfo", "VarInfo")
		}
	}
	units = append(units, Unit{Source: root + "dwarf/putvarabbrevgen.go", WholeFile: true, Context: emitter(map[string]string{"putvarAbbrevs": "putvarAbbrevs"})})
	units = append(units, Unit{Source: root + "obj/objfile.go", Functions: []string{"TrimInlineHash"}, Context: func() *ContextRule {
		c := objectDataContext()
		c.Fields = map[string]string{"inlHashLen": "inlHashLen"}
		return c
	}(), ContextFrom: map[string]ContextSource{"TrimInlineHash": {Argument: "name", Resolver: "inlineHashLink"}}})
	units = append(units, Unit{Source: root + "obj/pcln.go", WholeFile: true, Context: objectDataContext(), ContextFrom: map[string]ContextSource{"pctospadj": {Argument: "ctxt", Resolver: "dataLink"}, "pctopcdata": {Argument: "ctxt", Resolver: "dataLink"}, "PCIter.Next": {Argument: "it", Resolver: "pcIterLink"}}})
	units = compilerDwarfMigration(units)
	var ordered []Unit
	for _, pkg := range []string{"internal/goexperiment", "internal/buildcfg"} {
		for _, u := range units {
			if u.sourcePackage() == pkg {
				ordered = append(ordered, u)
			}
		}
	}
	for _, u := range units {
		if u.sourcePackage() != "internal/goexperiment" && u.sourcePackage() != "internal/buildcfg" {
			ordered = append(ordered, u)
		}
	}
	return ordered
}

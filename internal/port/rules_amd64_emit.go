package port

func amd64EmissionMigration(units []Unit) []Unit {
	const root = "src/cmd/compile/internal/"
	context := func(fields map[string]string) *ContextRule {
		return &ContextRule{Type: "Context", Constructor: "initialize", Initialize: true, Selective: true, RedirectTrace: true, Adapter: "adapter_context.go", Fields: fields}
	}
	states := []StateRule{
		{Source: root + "ir/symtab.go", Variable: "Syms", Field: "Backend.Typecheck.IR.Syms"},
		{Source: root + "base/link.go", Variable: "Ctxt", Field: "Backend.Typecheck.IR.Base.Ctxt"},
		{Source: root + "base/debug.go", Variable: "Debug", Field: "Backend.Typecheck.IR.Debug"},
		{Source: root + "types/size.go", Variable: "PtrSize", Field: "ptrSize"},
		{Source: root + "ssa/rewrite.go", Variable: "AuxMark", Field: "Backend.SSA.AuxMark"},
		{Source: "src/cmd/internal/obj/x86/obj6.go", Variable: "Linkamd64", Field: "LinkAMD64"},
	}
	calls := []CallRule{
		{Source: "src/cmd/internal/obj/x86/obj6.go", Function: "CanUse1InsnTLS", Method: "x86.CanUse1InsnTLS"},
		{Source: "src/cmd/internal/obj/x86/evex.go", Function: "ParseSuffix", Method: "x86.ParseSuffix"},
		{Source: root + "base/print.go", Function: "Fatalf", Method: "fatalf"},
		{Source: root + "base/print.go", Function: "WarnfAt", Method: "warnfAt"},
		{Source: root + "logopt/log_opts.go", Function: "Enabled", Method: "Backend.SSA.Log.Enabled"},
		{Source: root + "logopt/log_opts.go", Function: "LogOpt", Method: "Backend.SSA.Log.LogOpt"},
	}
	for _, n := range []string{"AddrAuto", "CheckArgReg", "CheckLoweredGetClosurePtr"} {
		calls = append(calls, CallRule{Source: root + "ssagen/ssa.go", Function: n, Method: "Backend." + n})
	}
	units = append(units,
		Unit{Source: root + "amd64/galign.go", WholeFile: true, Context: context(map[string]string{"leaptr": "leaptr"}), State: states, Calls: calls},
		Unit{Source: root + "amd64/ssa.go", WholeFile: true, Context: context(map[string]string{"blockJump": "blockJump", "eqfJumps": "eqfJumps", "nefJumps": "nefJumps"}), State: states, Calls: calls},
		Unit{Source: root + "amd64/simdssa.go", WholeFile: true, Context: context(nil), State: states, Calls: calls},
		Unit{Source: root + "amd64/ggen.go", WholeFile: true, Context: context(nil), State: states, Calls: calls},
	)
	// Native x86 formatter initialization becomes registration on this Link.
	xctx := func(fields map[string]string) *ContextRule {
		return &ContextRule{Type: "Context", Constructor: "initialize", Initialize: true, Init: true, Selective: true, Adapter: "adapter_context.go", Fields: fields}
	}
	xcalls := []CallRule{}
	for _, n := range []string{"RegisterRegister", "RegisterOpcode", "RegisterRegisterList", "RegisterOpSuffix"} {
		xcalls = append(xcalls, CallRule{Source: "src/cmd/internal/obj/util.go", Function: n, Method: "Link." + n})
	}
	units = append(units,
		Unit{Source: "src/cmd/internal/obj/x86/list6.go", WholeFile: true, Context: xctx(nil), ReadTables: []string{"Register"}, Calls: xcalls},
		Unit{Source: "src/cmd/internal/obj/x86/anames.go", WholeFile: true, Context: xctx(map[string]string{"Anames": "Anames"})},
		Unit{Source: "src/cmd/internal/obj/x86/evex.go", Functions: []string{"decodeRegisterRange", "EncodeRegisterRange", "newEVEXSuffix", "ParseSuffix", "newOpSuffix", "inferSuffixError"}, Declarations: []string{"opSuffix", "opSuffix.String", "opSuffix.IsValid", "badOpSuffix", "evexSuffix", "rcRNSAE"}, Context: xctx(map[string]string{"opSuffixTable": "opSuffixTable", "evexSuffixMap": "evexSuffixMap"}), ContextParams: []string{"opSuffix.String"}},
	)
	units = append(units, Unit{Source: "src/cmd/internal/obj/x86/obj6.go", Functions: []string{"CanUse1InsnTLS"}, Context: xctx(nil)})
	units = append(units, Unit{Source: "src/cmd/internal/obj/x86/asm6.go", Context: xctx(map[string]string{"isAndroid": "isAndroid"}), State: []StateRule{{Source: "src/internal/buildcfg/cfg.go", Variable: "GOOS", Field: "Link.TargetOS"}}})
	for i := range units {
		if units[i].Source == "src/cmd/internal/obj/x86/a.out.go" {
			units[i].Context = xctx(map[string]string{"AMD64DWARFRegisters": "AMD64DWARFRegisters"})
		}
	}
	return units
}

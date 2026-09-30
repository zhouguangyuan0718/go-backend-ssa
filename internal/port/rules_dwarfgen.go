package port

func dwarfgenMigration() []Unit {
	context := func() *ContextRule {
		return &ContextRule{Type: "Context", Constructor: "newContext", Adapter: "adapter_context.go", Selective: true}
	}
	return []Unit{
		{Source: "src/cmd/compile/internal/dwarfgen/marker.go", WholeFile: true, Context: context(), Owner: &OwnerRule{Types: []string{"ScopeMarker"}, Context: "Context", Field: "compilation"}, Calls: []CallRule{
			{Source: "src/cmd/compile/internal/base/print.go", Function: "Fatalf", Method: "fatalf"},
			{Source: "src/cmd/compile/internal/base/print.go", Function: "FatalfAt", Method: "fatalfAt"},
			{Source: "src/cmd/compile/internal/base/print.go", Function: "FmtPos", Method: "Base.FmtPos"},
		}},
		{Source: "src/cmd/compile/internal/dwarfgen/scope.go", Functions: []string{"xposBefore"}, Context: context(), State: []StateRule{{Source: "src/cmd/compile/internal/base/link.go", Variable: "Ctxt", Field: "Base.Ctxt"}}},
	}
}

// Compiler DWARF callbacks retain the native scope, variable and inline passes.
func compilerDwarfMigration(units []Unit) []Unit {
	const root = "src/cmd/compile/internal/"
	for _, file := range []string{"dwarf.go", "dwinl.go"} {
		units = append(units, Unit{Source: root + "dwarfgen/" + file, WholeFile: true})
	}
	for i := range units {
		u := &units[i]
		if u.sourcePackage() != "cmd/compile/internal/dwarfgen" {
			continue
		}
		u.WholeFile = true
		u.Functions = nil
		u.Context = &ContextRule{Type: "Context", Constructor: "newContext", Adapter: "adapter_context.go", Selective: true, RedirectTrace: true}
		u.State = []StateRule{
			{Source: root + "ssa/debug.go", Variable: "BlockStart", Field: "Backend.SSA.BlockStart"},
			{Source: root + "ssa/debug.go", Variable: "FuncEnd", Field: "Backend.SSA.FuncEnd"},
			{Source: root + "base/link.go", Variable: "Ctxt", Field: "Base.Ctxt"},
			{Source: root + "base/flag.go", Variable: "Flag", Field: "Base.Flag"},
			{Source: root + "base/debug.go", Variable: "Debug", Field: "Backend.Typecheck.IR.Debug"},
			{Source: root + "types/size.go", Variable: "PtrSize", Field: "ptrSize"},
			{Source: root + "types/fmt.go", Variable: "LocalPkg", Field: "Backend.Typecheck.IR.Types.LocalPkg"},
			{Source: "src/internal/buildcfg/exp.go", Variable: "Experiment", Field: "Backend.Typecheck.IR.Types.Experiment"},
			{Source: "src/internal/buildcfg/exp.go", Variable: "FramePointerEnabled", Field: "Backend.SSA.FramePointerEnabled"},
		}
		u.Calls = []CallRule{
			{Source: root + "base/print.go", Function: "Fatalf", Method: "fatalf"},
			{Source: root + "base/print.go", Function: "FatalfAt", Method: "fatalfAt"},
			{Source: root + "base/print.go", Function: "FmtPos", Method: "Base.FmtPos"},
			{Source: root + "types/size.go", Function: "CalcSize", Method: "Backend.Typecheck.IR.Types.CalcSize"},
			{Source: root + "types/type.go", Function: "TypeSymName", Method: "Backend.Typecheck.IR.Types.TypeSymName"},
			{Source: root + "reflectdata/reflect.go", Function: "TypeLinksym", Method: "ReflectData.TypeLinksym"},
			{Source: root + "typecheck/func.go", Function: "NewClosureStructIter", Method: "Backend.Typecheck.NewClosureStructIter"},
			{Source: root + "ssagen/pgen.go", Function: "StackOffset", Method: "Backend.StackOffset"},
		}
		if u.Source == root+"dwarfgen/dwarf.go" {
			u.HostCalls = []HostCallRule{{Package: "flag", Function: "Lookup", Method: "lookupFlag"}}
			u.Checks = map[string]string{"Info": "checkInfo"}
		}
	}
	// The complete callbacks depend on SSA and ssagen. Their frontend consumers
	// must follow them, while the backend itself has no dependency on those passes.
	delayed := map[string]bool{"cmd/compile/internal/dwarfgen": true, "cmd/compile/internal/noder": true, "cmd/compile/internal/pkginit": true, "cmd/compile/internal/walk": true}
	var out []Unit
	for _, u := range units {
		if !delayed[u.sourcePackage()] {
			out = append(out, u)
		}
	}
	for _, pkg := range []string{"cmd/compile/internal/dwarfgen", "cmd/compile/internal/noder", "cmd/compile/internal/pkginit", "cmd/compile/internal/walk"} {
		for _, u := range units {
			if u.sourcePackage() == pkg {
				out = append(out, u)
			}
		}
	}
	return out
}

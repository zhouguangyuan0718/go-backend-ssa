package port

func typecheckMigration() []Unit {
	context := func() *ContextRule {
		return &ContextRule{Type: "Context", Constructor: "initialize", Initialize: true, Selective: true, PreserveNilArgs: true, Adapter: "adapter_context.go"}
	}
	universe := Unit{Source: "src/cmd/compile/internal/typecheck/universe.go", WholeFile: true, Context: context()}
	universe.Context.Fields = map[string]string{}
	for _, name := range []string{"okfor", "okforeq", "okforadd", "okforand", "okfornone", "okforbool", "okforcap", "okforlen", "okforarith", "builtinFuncs", "unsafeFuncs"} {
		universe.Context.Fields[name] = name
	}
	for _, name := range []string{"IsInt", "IsFloat", "IsComplex", "IsOrdered", "IsSimple", "Types", "UntypedBool", "ByteType", "RuneType"} {
		universe.State = append(universe.State, StateRule{Source: "src/cmd/compile/internal/types/type.go", Variable: name, Field: "IR.Types." + name})
	}
	for _, name := range []string{"BuiltinPkg", "UnsafePkg", "BlankSym", "LocalPkg"} {
		universe.State = append(universe.State, StateRule{Source: "src/cmd/compile/internal/types/fmt.go", Variable: name, Field: "IR.Types." + name})
	}
	universe.State = append(universe.State,
		StateRule{Source: "src/cmd/compile/internal/base/print.go", Variable: "Pos", Field: "IR.Types.Pos"},
		StateRule{Source: "src/cmd/compile/internal/ir/node.go", Variable: "BlankNode", Field: "IR.BlankNode"},
		StateRule{Source: "src/cmd/compile/internal/ir/val.go", Variable: "OKForConst", Field: "IR.OKForConst"},
		StateRule{Source: "src/cmd/compile/internal/ir/func.go", Variable: "CurFunc", Field: "IR.CurFunc"},
		StateRule{Source: "src/cmd/compile/internal/ir/symtab.go", Variable: "Pkgs", Field: "IR.Pkgs"},
		StateRule{Source: "src/cmd/compile/internal/base/timings.go", Variable: "Timer", Field: "IR.Base.Timer"},
	)
	for _, name := range []string{"NewDeclNameAt", "NewBuiltin", "NewConstAt", "NewNameAt"} {
		universe.Calls = append(universe.Calls, CallRule{Source: "src/cmd/compile/internal/ir/name.go", Function: name, Method: "IR." + name})
	}
	universe.Calls = append(universe.Calls,
		CallRule{Source: "src/cmd/compile/internal/types/universe.go", Function: "InitTypes", Method: "IR.Types.InitTypes"},
		CallRule{Source: "src/cmd/compile/internal/types/type.go", Function: "NewField", Method: "IR.Types.NewField"},
		CallRule{Source: "src/cmd/compile/internal/ir/expr.go", Function: "NewNilExpr", Method: "IR.NewNilExpr"},
		CallRule{Source: "src/cmd/compile/internal/base/print.go", Function: "Fatalf", Method: "fatalf"},
	)
	for _, name := range []string{"NewSignature", "NewPtr", "NewArray", "NewSlice", "NewMap", "NewChan", "NewStruct", "SubstAny"} {
		universe.Calls = append(universe.Calls, CallRule{Source: "src/cmd/compile/internal/types/type.go", Function: name, Method: "IR.Types." + name})
	}
	universe.Calls = append(universe.Calls,
		CallRule{Source: "src/cmd/compile/internal/ir/func.go", Function: "NewFunc", Method: "IR.NewFunc"},
		CallRule{Source: "src/cmd/compile/internal/types/size.go", Function: "CalcSize", Method: "IR.Types.CalcSize"},
		CallRule{Source: "src/cmd/compile/internal/base/link.go", Function: "PkgLinksym", Method: "IR.Base.PkgLinksym"},
	)
	builtin := Unit{Source: "src/cmd/compile/internal/typecheck/builtin.go", WholeFile: true, Context: context()}
	builtin.Context.Fields = map[string]string{"runtimeDecls": "runtimeDecls", "coverageDecls": "coverageDecls"}
	return []Unit{
		builtin,
		{Source: "src/cmd/compile/internal/typecheck/bexport.go", WholeFile: true},
		{Source: "src/cmd/compile/internal/typecheck/export.go", WholeFile: true, Context: context()},
		universe,
		{Source: "src/cmd/compile/internal/typecheck/subr.go", Functions: []string{"NodNil", "NewFuncParams", "LookupNum"}, Context: context()},
		{Source: "src/cmd/compile/internal/typecheck/syms.go", WholeFile: true, Context: context()},
	}
}

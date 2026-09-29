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
	for _, name := range []string{"IsInt", "IsFloat", "IsComplex", "IsOrdered", "IsSimple", "Types", "UntypedBool"} {
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
	return []Unit{
		universe,
		{Source: "src/cmd/compile/internal/typecheck/subr.go", Functions: []string{"NodNil", "NewFuncParams", "LookupNum"}, Context: context()},
		{Source: "src/cmd/compile/internal/typecheck/syms.go", Functions: []string{"Lookup", "AutoLabel"}, Context: context()},
	}
}

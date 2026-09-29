package port

func typecheckMigration() []Unit {
	context := func() *ContextRule {
		return &ContextRule{Type: "Context", Constructor: "initialize", Initialize: true, Selective: true, PreserveNilArgs: true, RedirectTrace: true, Adapter: "adapter_context.go"}
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
	for _, name := range []string{"ComplexForFloat", "ReceiverBaseType", "RuntimeSymName", "IsMethodApplicable", "NewSignature", "NewPtr", "NewArray", "NewSlice", "NewMap", "NewChan", "NewStruct", "SubstAny"} {
		universe.Calls = append(universe.Calls, CallRule{Source: "src/cmd/compile/internal/types/type.go", Function: name, Method: "IR.Types." + name})
	}
	universe.Calls = append(universe.Calls,
		CallRule{Source: "src/cmd/compile/internal/ir/func.go", Function: "NewFunc", Method: "IR.NewFunc"},
		CallRule{Source: "src/cmd/compile/internal/types/size.go", Function: "CalcSize", Method: "IR.Types.CalcSize"},
		CallRule{Source: "src/cmd/compile/internal/base/link.go", Function: "PkgLinksym", Method: "IR.Base.PkgLinksym"},
	)
	for _, name := range []string{"IncomparableField", "IsComparable"} {
		universe.Calls = append(universe.Calls, CallRule{Source: "src/cmd/compile/internal/types/alg.go", Function: name, Method: "IR.Types." + name})
	}
	for _, name := range []string{"Identical", "IdenticalIgnoreTags", "IdenticalStrict"} {
		universe.Calls = append(universe.Calls, CallRule{Source: "src/cmd/compile/internal/types/identity.go", Function: name, Method: "IR.Types." + name})
	}
	for _, name := range []string{"CheckSize", "RoundUp"} {
		universe.Calls = append(universe.Calls, CallRule{Source: "src/cmd/compile/internal/types/size.go", Function: name, Method: "IR.Types." + name})
	}
	for _, name := range []string{"CompareFields", "FloatForComplex"} {
		universe.Calls = append(universe.Calls, CallRule{Source: "src/cmd/compile/internal/types/type.go", Function: name, Method: "IR.Types." + name})
	}
	for _, name := range []string{"BigFloat", "NewInt"} {
		universe.Calls = append(universe.Calls, CallRule{Source: "src/cmd/compile/internal/ir/const.go", Function: name, Method: "IR." + name})
	}
	for _, name := range []string{"NewAddStringExpr", "NewAddrExpr", "NewBinaryExpr", "NewCallExpr", "NewCompLitExpr", "NewConstExpr", "NewConvExpr", "NewKeyExpr", "NewLinksymExpr", "NewMakeExpr", "NewSelectorExpr", "NewStarExpr", "NewStructKeyExpr", "NewUnaryExpr", "StaticCalleeName"} {
		universe.Calls = append(universe.Calls, CallRule{Source: "src/cmd/compile/internal/ir/expr.go", Function: name, Method: "IR." + name})
	}
	for _, name := range []string{"Dump"} {
		universe.Calls = append(universe.Calls, CallRule{Source: "src/cmd/compile/internal/ir/fmt.go", Function: name, Method: "IR." + name})
	}
	for _, name := range []string{"NewClosureFunc"} {
		universe.Calls = append(universe.Calls, CallRule{Source: "src/cmd/compile/internal/ir/func.go", Function: name, Method: "IR." + name})
	}
	for _, name := range []string{"DeclaredBy", "NewClosureVar"} {
		universe.Calls = append(universe.Calls, CallRule{Source: "src/cmd/compile/internal/ir/name.go", Function: name, Method: "IR." + name})
	}
	for _, name := range []string{"HasUniquePos", "InitExpr", "Line", "SetPos"} {
		universe.Calls = append(universe.Calls, CallRule{Source: "src/cmd/compile/internal/ir/node.go", Function: name, Method: "IR." + name})
	}
	for _, name := range []string{"NewAssignListStmt", "NewAssignStmt", "NewBlockStmt", "NewDecl"} {
		universe.Calls = append(universe.Calls, CallRule{Source: "src/cmd/compile/internal/ir/stmt.go", Function: name, Method: "IR." + name})
	}
	for _, name := range []string{"TypeNode"} {
		universe.Calls = append(universe.Calls, CallRule{Source: "src/cmd/compile/internal/ir/type.go", Function: name, Method: "IR." + name})
	}
	for _, name := range []string{"IntVal", "StringVal"} {
		universe.Calls = append(universe.Calls, CallRule{Source: "src/cmd/compile/internal/ir/val.go", Function: name, Method: "IR." + name})
	}

	for _, name := range []string{"Errorf", "ErrorfAt", "FatalfAt", "Assertf", "AssertfAt", "Errors", "UpdateErrorDot"} {
		method := map[string]string{"Errorf": "errorf", "ErrorfAt": "errorfAt", "FatalfAt": "fatalfAt", "Assertf": "assertf", "AssertfAt": "assertfAt", "Errors": "errors", "UpdateErrorDot": "updateErrorDot"}[name]
		universe.Calls = append(universe.Calls, CallRule{Source: "src/cmd/compile/internal/base/print.go", Function: name, Method: method})
	}
	universe.Calls = append(universe.Calls, CallRule{Source: "src/cmd/compile/internal/base/print.go", Function: "FmtPos", Method: "IR.Base.FmtPos"})
	universe.State = append(universe.State,
		StateRule{Source: "src/cmd/compile/internal/base/print.go", Variable: "AutogeneratedPos", Field: "IR.Types.AutogeneratedPos"},
		StateRule{Source: "src/cmd/compile/internal/base/flag.go", Variable: "Flag", Field: "IR.Base.Flag"},
		StateRule{Source: "src/cmd/compile/internal/types/size.go", Variable: "PtrSize", Field: "ptrSize"},
		StateRule{Source: "src/cmd/compile/internal/types/size.go", Variable: "SkipSizeForTracing", Field: "IR.Types.SkipSizeForTracing"},
	)

	universe.State = append(universe.State, StateRule{Source: "src/cmd/compile/internal/types/type.go", Variable: "UntypedString", Field: "IR.Types.UntypedString"})
	universe.State = append(universe.State, StateRule{Source: "src/cmd/compile/internal/types/type.go", Variable: "UntypedInt", Field: "IR.Types.UntypedInt"})
	universe.State = append(universe.State, StateRule{Source: "src/cmd/compile/internal/types/type.go", Variable: "UntypedRune", Field: "IR.Types.UntypedRune"})
	universe.State = append(universe.State, StateRule{Source: "src/cmd/compile/internal/types/type.go", Variable: "UntypedFloat", Field: "IR.Types.UntypedFloat"})
	universe.State = append(universe.State, StateRule{Source: "src/cmd/compile/internal/types/type.go", Variable: "UntypedComplex", Field: "IR.Types.UntypedComplex"})
	universe.State = append(universe.State, StateRule{Source: "src/cmd/compile/internal/types/type.go", Variable: "SimType", Field: "IR.Types.SimType"})
	builtin := Unit{Source: "src/cmd/compile/internal/typecheck/builtin.go", WholeFile: true, Context: context()}
	builtin.Context.Fields = map[string]string{"runtimeDecls": "runtimeDecls", "coverageDecls": "coverageDecls"}
	units := []Unit{
		builtin,
		{Source: "src/cmd/compile/internal/typecheck/bexport.go", WholeFile: true},
		{Source: "src/cmd/compile/internal/typecheck/export.go", WholeFile: true, Context: context()},
		universe,

		{Source: "src/cmd/compile/internal/typecheck/syms.go", WholeFile: true, Context: context()},
	}
	for _, file := range []string{"const", "dcl", "expr", "func", "iexport", "iimport", "stmt", "subr", "target", "type", "typecheck"} {
		u := Unit{Source: "src/cmd/compile/internal/typecheck/" + file + ".go", WholeFile: true, Context: context()}
		switch file {
		case "typecheck":
			u.ReadTables = []string{"_typekind"}
			u.Context.Fields = map[string]string{"traceIndent": "traceIndent"}
		case "dcl":
			u.Context.Fields = map[string]string{"funcStack": "funcStack", "autotmpnamesmu": "autotmpnamesmu", "autotmpnames": "autotmpnames"}
		case "subr":
			u.Context.Fields = map[string]string{"dotlist": "dotlist", "slist": "slist"}
		case "target":
			u.Context.Fields = map[string]string{"Target": "Target"}
		case "iimport":
			u.Context.Fields = map[string]string{"HaveInlineBody": "HaveInlineBody", "typeSymIdx": "typeSymIdx"}
		case "stmt":
			u.Owner = &OwnerRule{Types: []string{"typeSet"}, Context: "Context", Field: "compilation"}
		case "func":
			u.Owner = &OwnerRule{Types: []string{"ClosureStructIter"}, Context: "Context", Field: "compilation"}
		}
		units = append(units, u)
	}
	return units
}

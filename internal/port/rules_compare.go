package port

func compareMigration() []Unit {
	const root = "src/cmd/compile/internal/"
	u := Unit{Source: root + "compare/compare.go", WholeFile: true,
		Context: &ContextRule{Type: "Context", Constructor: "newContext", Adapter: "adapter_context.go", Selective: true},
		ContextFrom: map[string]ContextSource{
			"IsRegularMemory":      {Argument: "t", Resolver: "typeContextOf"},
			"Memrun":               {Argument: "t", Resolver: "typeContextOf"},
			"EqStructCost":         {Argument: "t", Resolver: "typeContextOf"},
			"eqStructFieldCost":    {Argument: "t", Resolver: "typeContextOf"},
			"calculateCostForType": {Argument: "t", Resolver: "typeContextOf"},
		},
		Checks: map[string]string{"Memrun": "checkMemrun", "EqStructCost": "checkCost", "EqStruct": "checkStruct", "EqString": "checkNodes", "EqInterface": "checkNodes"},
		State: []StateRule{
			{Source: root + "base/link.go", Variable: "Ctxt", Field: "Ctxt"},
			{Source: root + "base/print.go", Variable: "Pos", Field: "Types.Pos"},
			{Source: root + "types/type.go", Variable: "Types", Field: "Types.Types"},
			{Source: root + "types/size.go", Variable: "RegSize", Field: "regSize"},
		},
		Calls: []CallRule{{Source: root + "base/print.go", Function: "Fatalf", Method: "fatalf"}},
	}
	for _, group := range []struct {
		file, prefix string
		names        []string
	}{
		{"types/alg.go", "Types.", []string{"AlgType", "IsPaddedField"}},
		{"types/identity.go", "Types.", []string{"Identical"}},
		{"ir/expr.go", "Typecheck.IR.", []string{"NewConvExpr", "NewUnaryExpr", "NewBinaryExpr", "NewCallExpr"}},
		{"ir/const.go", "Typecheck.IR.", []string{"NewInt"}},
		{"typecheck/typecheck.go", "Typecheck.", []string{"Expr", "Conv", "Call"}},
		{"typecheck/expr.go", "Typecheck.", []string{"DotField"}},
		{"typecheck/syms.go", "Typecheck.", []string{"LookupRuntime"}},
		{"typecheck/subr.go", "Typecheck.", []string{"NodAddr"}},
	} {
		for _, name := range group.names {
			u.Calls = append(u.Calls, CallRule{Source: root + group.file, Function: name, Method: group.prefix + name})
		}
	}
	return []Unit{u}
}

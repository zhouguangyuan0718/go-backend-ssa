package port

func irMigration() []Unit {
	units := []Unit{
		{Source: "src/cmd/compile/internal/ir/scc.go", WholeFile: true, Owner: &OwnerRule{Types: []string{"bottomUpVisitor"}, Context: "Context", Field: "compilation"}},
		{Source: "src/cmd/compile/internal/ir/symtab.go", WholeFile: true},
		{Source: "src/cmd/compile/internal/ir/mini.go",
			WholeFile: true,
			Owner:     &OwnerRule{Context: "Context", Field: "compilation", Types: []string{"miniNode"}, Embedded: map[string]string{}}},
		{Source: "src/cmd/compile/internal/ir/node.go",
			WholeFile: true},
		{Source: "src/cmd/compile/internal/ir/node_gen.go",
			WholeFile: true},
		{Source: "src/cmd/compile/internal/ir/expr.go",
			WholeFile:   true,
			Owner:       &OwnerRule{Context: "Context", Field: "compilation", Types: []string{"miniExpr", "AddStringExpr", "AddrExpr", "BasicLit", "BinaryExpr", "CallExpr", "ClosureExpr", "CompLitExpr", "ConvExpr", "IndexExpr", "KeyExpr", "StructKeyExpr", "InlinedCallExpr", "LogicalExpr", "MakeExpr", "NilExpr", "ParenExpr", "ResultExpr", "LinksymOffsetExpr", "SelectorExpr", "SliceExpr", "SliceHeaderExpr", "StringHeaderExpr", "StarExpr", "TypeAssertExpr", "DynamicTypeAssertExpr", "UnaryExpr", "MoveToHeapExpr"}, Embedded: map[string]string{"miniExpr": "miniNode", "AddStringExpr": "miniExpr.miniNode", "AddrExpr": "miniExpr.miniNode", "BasicLit": "miniExpr.miniNode", "BinaryExpr": "miniExpr.miniNode", "CallExpr": "miniExpr.miniNode", "ClosureExpr": "miniExpr.miniNode", "CompLitExpr": "miniExpr.miniNode", "ConvExpr": "miniExpr.miniNode", "IndexExpr": "miniExpr.miniNode", "KeyExpr": "miniExpr.miniNode", "StructKeyExpr": "miniExpr.miniNode", "InlinedCallExpr": "miniExpr.miniNode", "LogicalExpr": "miniExpr.miniNode", "MakeExpr": "miniExpr.miniNode", "NilExpr": "miniExpr.miniNode", "ParenExpr": "miniExpr.miniNode", "ResultExpr": "miniExpr.miniNode", "LinksymOffsetExpr": "miniExpr.miniNode", "SelectorExpr": "miniExpr.miniNode", "SliceExpr": "miniExpr.miniNode", "SliceHeaderExpr": "miniExpr.miniNode", "StringHeaderExpr": "miniExpr.miniNode", "StarExpr": "miniExpr.miniNode", "TypeAssertExpr": "miniExpr.miniNode", "DynamicTypeAssertExpr": "miniExpr.miniNode", "UnaryExpr": "miniExpr.miniNode", "MoveToHeapExpr": "miniExpr.miniNode"}},
			ContextFrom: map[string]ContextSource{"Op.IsSlice3": {Argument: "o", Resolver: "opContext"}}},
		{Source: "src/cmd/compile/internal/ir/stmt.go",
			WholeFile: true,
			Owner:     &OwnerRule{Context: "Context", Field: "compilation", Types: []string{"Decl", "miniStmt", "AssignListStmt", "AssignStmt", "AssignOpStmt", "BlockStmt", "BranchStmt", "CaseClause", "CommClause", "ForStmt", "GoDeferStmt", "IfStmt", "JumpTableStmt", "InterfaceSwitchStmt", "InlineMarkStmt", "LabelStmt", "RangeStmt", "ReturnStmt", "SelectStmt", "SendStmt", "SwitchStmt", "TailCallStmt", "TypeSwitchGuard"}, Embedded: map[string]string{"Decl": "miniNode", "miniStmt": "miniNode", "AssignListStmt": "miniStmt.miniNode", "AssignStmt": "miniStmt.miniNode", "AssignOpStmt": "miniStmt.miniNode", "BlockStmt": "miniStmt.miniNode", "BranchStmt": "miniStmt.miniNode", "CaseClause": "miniStmt.miniNode", "CommClause": "miniStmt.miniNode", "ForStmt": "miniStmt.miniNode", "GoDeferStmt": "miniStmt.miniNode", "IfStmt": "miniStmt.miniNode", "JumpTableStmt": "miniStmt.miniNode", "InterfaceSwitchStmt": "miniStmt.miniNode", "InlineMarkStmt": "miniStmt.miniNode", "LabelStmt": "miniStmt.miniNode", "RangeStmt": "miniStmt.miniNode", "ReturnStmt": "miniStmt.miniNode", "SelectStmt": "miniStmt.miniNode", "SendStmt": "miniStmt.miniNode", "SwitchStmt": "miniStmt.miniNode", "TailCallStmt": "miniStmt.miniNode", "TypeSwitchGuard": "miniNode"}}},
		{Source: "src/cmd/compile/internal/ir/name.go",
			WholeFile: true,
			Owner:     &OwnerRule{Context: "Context", Field: "compilation", Types: []string{"Ident", "Name"}, Embedded: map[string]string{"Ident": "miniExpr.miniNode", "Name": "miniExpr.miniNode"}}},
		{Source: "src/cmd/compile/internal/ir/func.go",
			WholeFile: true, Owner: &OwnerRule{Context: "Context", Field: "compilation", Types: []string{"Func"}, Embedded: map[string]string{"Func": "miniNode"}}},
		{Source: "src/cmd/compile/internal/ir/type.go",
			WholeFile: true,
			Owner:     &OwnerRule{Context: "Context", Field: "compilation", Types: []string{"typeNode", "DynamicType"}, Embedded: map[string]string{"typeNode": "miniNode", "DynamicType": "miniExpr.miniNode"}}},
		{Source: "src/cmd/compile/internal/ir/package.go",
			WholeFile: true},
		{Source: "src/cmd/compile/internal/ir/visit.go",
			WholeFile: true},
		{Source: "src/cmd/compile/internal/ir/copy.go",
			WholeFile: true},
		{Source: "src/cmd/compile/internal/ir/fmt.go",
			WholeFile:   true,
			ReadTables:  []string{"OpNames", "OpPrec"},
			ContextFrom: map[string]ContextSource{"fmtNode": {Argument: "n", Resolver: "nodeContext"}, "Nodes.Format": {Argument: "l", Resolver: "nodesContext"}}},
		{Source: "src/cmd/compile/internal/ir/bitset.go",
			WholeFile: true},
		{Source: "src/cmd/compile/internal/ir/op_string.go",
			WholeFile:  true,
			ReadTables: []string{"_Op_index"}},
		{Source: "src/cmd/compile/internal/ir/class_string.go",
			WholeFile:  true,
			ReadTables: []string{"_Class_index"}},
		{Source: "src/cmd/compile/internal/ir/const.go",
			WholeFile: true},
		{Source: "src/cmd/compile/internal/ir/val.go",
			WholeFile: true},
		{Source: "src/cmd/compile/internal/ir/cfg.go",
			WholeFile: true},
	}
	for i := range units {
		units[i].Context = &ContextRule{Type: "Context", Constructor: "initialize", Initialize: true, Selective: true, PreserveNilArgs: true, RedirectTrace: true, Adapter: "adapter_context.go"}
	}
	fields := map[string][]string{
		"symtab.go": {"Syms", "Pkgs"},
		"node.go":   {"BlankNode"}, "expr.go": {"IsIntrinsicCall", "IsIntrinsicSym"},
		"func.go": {"CurFunc", "globClosgen"}, "fmt.go": {"EscFmt", "nodeType"},
		"const.go": {"intZero", "floatZero", "complexZero", "intOne", "floatOne", "complexOne"},
		"val.go":   {"OKForConst"}, "cfg.go": {"MaxStackVarSize", "MaxImplicitStackVarSize", "MaxSmallArraySize"},
	}
	for i := range units {
		for file, names := range fields {
			if units[i].Source == "src/cmd/compile/internal/ir/"+file {
				units[i].Context.Fields = map[string]string{}
				for _, n := range names {
					units[i].Context.Fields[n] = n
				}
			}
		}
	}
	units[0].State = []StateRule{
		{Source: "src/cmd/compile/internal/base/link.go", Variable: "Ctxt", Field: "Base.Ctxt"},
		{Source: "src/cmd/compile/internal/base/flag.go", Variable: "Flag", Field: "Base.Flag"},
		{Source: "src/cmd/compile/internal/base/debug.go", Variable: "Debug", Field: "Debug"},
		{Source: "src/cmd/compile/internal/base/print.go", Variable: "Pos", Field: "Types.Pos"},
	}
	for _, n := range []string{"Types", "IsSimple", "DefaultKinds", "LocalPkg", "RuneType", "UntypedBool", "UntypedInt", "UntypedRune", "UntypedString", "UntypedTypes"} {
		source := "src/cmd/compile/internal/types/type.go"
		if n == "LocalPkg" {
			source = "src/cmd/compile/internal/types/fmt.go"
		}
		units[0].State = append(units[0].State, StateRule{Source: source, Variable: n, Field: "Types." + n})
	}
	for _, n := range []string{"Fatalf", "FatalfAt", "ErrorfAt", "Warn", "WarnfAt"} {
		method := map[string]string{"Fatalf": "fatalf", "FatalfAt": "fatalfAt", "ErrorfAt": "errorfAt", "Warn": "warn", "WarnfAt": "warnfAt"}[n]
		units[0].Calls = append(units[0].Calls, CallRule{Source: "src/cmd/compile/internal/base/print.go", Function: n, Method: method})
	}
	units[0].Calls = append(units[0].Calls,
		CallRule{Source: "src/cmd/compile/internal/base/print.go", Function: "FmtPos", Method: "Base.FmtPos"},
		CallRule{Source: "src/cmd/internal/obj/objfile.go", Function: "TrimInlineHash", Method: "trimInlineHash"},
		CallRule{Source: "src/cmd/compile/internal/types/type.go", Function: "NewPtr", Method: "Types.NewPtr"},
		CallRule{Source: "src/cmd/compile/internal/types/type.go", Function: "FloatForComplex", Method: "Types.FloatForComplex"},
		CallRule{Source: "src/cmd/compile/internal/types/identity.go", Function: "Identical", Method: "Types.Identical"},
		CallRule{Source: "src/cmd/compile/internal/types/pkg.go", Function: "NewPkg", Method: "Types.NewPkg"},
	)
	return units
}

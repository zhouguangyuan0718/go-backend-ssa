package port

// types2Migration retains the complete upstream type checker. Pure helpers keep
// their original signatures; private helper receivers receive context only when
// the typed dependency analysis proves it is needed.
func types2Migration() []Unit {
	context := func(runInit bool, names ...string) *ContextRule {
		fields := map[string]string{}
		for _, name := range names {
			fields[name] = name
		}
		return &ContextRule{Existing: true, Selective: true, PreserveNilArgs: true, Type: "Context", Constructor: "NewContext", Adapter: "adapter_context.go", Embed: "contextIO", RedirectTrace: true, Init: runInit, Fields: fields}
	}
	owner := func(names ...string) *OwnerRule {
		return &OwnerRule{Types: names, Context: "Context", Field: "compilation"}
	}
	return []Unit{
		{Source: "src/internal/types/errors/code_string.go", WholeFile: true, ReadTables: []string{"_Code_index_1", "_Code_index_2", "_Code_index_3", "_Code_index_4", "_Code_index_5"}},
		{Source: "src/internal/goversion/goversion.go", WholeFile: true},
		{Source: "src/cmd/compile/internal/types2/alias.go", WholeFile: true, Context: context(false), Owner: owner("Alias"), ContextParams: []string{"Checker.*"}},
		{Source: "src/cmd/compile/internal/types2/api.go", WholeFile: true, Context: context(false), Owner: &OwnerRule{Existing: true, Context: "Context", Field: "Context", Types: []string{"Config"}}, ContextParams: []string{"Info.*"}},
		{Source: "src/cmd/compile/internal/types2/api_predicates.go", WholeFile: true, Context: context(false)},
		{Source: "src/cmd/compile/internal/types2/array.go", WholeFile: true, Context: context(false), Owner: owner("Array")},
		{Source: "src/cmd/compile/internal/types2/assignments.go", WholeFile: true, Context: context(false), ContextParams: []string{"Checker.*"}},
		{Source: "src/cmd/compile/internal/types2/basic.go", WholeFile: true, Context: context(false), Owner: owner("Basic")},
		{Source: "src/cmd/compile/internal/types2/builtins.go", WholeFile: true, Context: context(false), ContextParams: []string{"Checker.*"}},
		{Source: "src/cmd/compile/internal/types2/call.go", WholeFile: true, Context: context(false, "cgoPrefixes"), ContextParams: []string{"Checker.*"}},
		{Source: "src/cmd/compile/internal/types2/chan.go", WholeFile: true, Context: context(false), Owner: owner("Chan")},
		{Source: "src/cmd/compile/internal/types2/check.go", WholeFile: true, Context: context(false), Owner: &OwnerRule{Existing: true, Context: "Context", Field: "ctxt", Types: []string{"Checker"}}, ContextParams: []string{"Checker.*", "action.*", "environment.*"}, ZeroValues: []string{"nopos"}},
		{Source: "src/cmd/compile/internal/types2/compiler_internal.go", WholeFile: true, Context: context(false)},
		{Source: "src/cmd/compile/internal/types2/compilersupport.go", WholeFile: true, Context: context(false)},
		{Source: "src/cmd/compile/internal/types2/const.go", WholeFile: true, Context: context(false), ContextParams: []string{"Checker.*"}},
		{Source: "src/cmd/compile/internal/types2/context.go", WholeFile: true, Context: context(false)},
		{Source: "src/cmd/compile/internal/types2/conversions.go", WholeFile: true, Context: context(false), ContextParams: []string{"Checker.*"}},
		{Source: "src/cmd/compile/internal/types2/cycles.go", WholeFile: true, Context: context(false), ContextParams: []string{"Checker.*"}},
		{Source: "src/cmd/compile/internal/types2/decl.go", WholeFile: true, Context: context(false), ContextParams: []string{"Checker.*"}},
		{Source: "src/cmd/compile/internal/types2/errors.go", WholeFile: true, Context: context(false), ContextParams: []string{"Checker.*", "error_.*"}},
		{Source: "src/cmd/compile/internal/types2/errsupport.go", WholeFile: true, Context: context(false), ContextParams: []string{"Checker.*"}},
		{Source: "src/cmd/compile/internal/types2/expr.go", WholeFile: true, Context: context(true, "binaryOpPredicates", "op2str1", "op2str2", "op2tok", "unaryOpPredicates"), ContextParams: []string{"Checker.*"}},
		{Source: "src/cmd/compile/internal/types2/format.go", WholeFile: true, Context: context(false), ContextParams: []string{"Checker.*"}},
		{Source: "src/cmd/compile/internal/types2/gccgosizes.go", WholeFile: true, Context: context(false, "gccgoArchSizes")},
		{Source: "src/cmd/compile/internal/types2/gcsizes.go", WholeFile: true, Context: context(false)},
		{Source: "src/cmd/compile/internal/types2/index.go", WholeFile: true, Context: context(false), ContextParams: []string{"Checker.*"}},
		{Source: "src/cmd/compile/internal/types2/infer.go", WholeFile: true, Context: context(false), ContextParams: []string{"Checker.*", "cycleFinder.*", "tpWalker.*"}},
		{Source: "src/cmd/compile/internal/types2/initorder.go", WholeFile: true, Context: context(false), ContextParams: []string{"Checker.*", "graphNode.*", "nodeSet.*"}},
		{Source: "src/cmd/compile/internal/types2/instantiate.go", WholeFile: true, Context: context(false), ContextParams: []string{"Checker.*"}},
		{Source: "src/cmd/compile/internal/types2/interface.go", WholeFile: true, Context: context(false, "emptyInterface"), Owner: owner("Interface"), ContextParams: []string{"Checker.*"}},
		{Source: "src/cmd/compile/internal/types2/labels.go", WholeFile: true, Context: context(false), ContextParams: []string{"Checker.*", "block.*"}},
		{Source: "src/cmd/compile/internal/types2/literals.go", WholeFile: true, Context: context(false), ContextParams: []string{"Checker.*"}},
		{Source: "src/cmd/compile/internal/types2/lookup.go", WholeFile: true, Context: context(false), ContextParams: []string{"Checker.*", "instanceLookup.*"}},
		{Source: "src/cmd/compile/internal/types2/map.go", WholeFile: true, Context: context(false), Owner: owner("Map")},
		{Source: "src/cmd/compile/internal/types2/mono.go", WholeFile: true, Context: context(false), ContextParams: []string{"Checker.*", "monoGraph.*"}},
		{Source: "src/cmd/compile/internal/types2/named.go", WholeFile: true, Context: context(false), Owner: owner("Named"), ContextParams: []string{"Checker.*"}},
		{Source: "src/cmd/compile/internal/types2/object.go", WholeFile: true, Context: context(false), Owner: owner("PkgName", "Const", "TypeName", "Var", "Func", "Label", "Builtin", "Nil"), ContextParams: []string{"object.*"}, ReadTables: []string{"varKindNames"}},
		{Source: "src/cmd/compile/internal/types2/objset.go", WholeFile: true, Context: context(false), ContextParams: []string{"objset.*"}},
		{Source: "src/cmd/compile/internal/types2/operand.go", WholeFile: true, Context: context(false, "operandModeString"), Owner: owner("operand")},
		{Source: "src/cmd/compile/internal/types2/package.go", WholeFile: true, Context: context(false), Owner: owner("Package")},
		{Source: "src/cmd/compile/internal/types2/pointer.go", WholeFile: true, Context: context(false), Owner: owner("Pointer")},
		{Source: "src/cmd/compile/internal/types2/predicates.go", WholeFile: true, Context: context(false), ContextParams: []string{"comparer.*", "ifacePair.*"}},
		{Source: "src/cmd/compile/internal/types2/range.go", WholeFile: true, Context: context(false), ContextParams: []string{"Checker.*"}},
		{Source: "src/cmd/compile/internal/types2/recording.go", WholeFile: true, Context: context(false), ContextParams: []string{"Checker.*"}},
		{Source: "src/cmd/compile/internal/types2/resolver.go", WholeFile: true, Context: context(false), ContextParams: []string{"Checker.*", "declInfo.*"}},
		{Source: "src/cmd/compile/internal/types2/return.go", WholeFile: true, Context: context(false), ContextParams: []string{"Checker.*"}},
		{Source: "src/cmd/compile/internal/types2/scope.go", WholeFile: true, Context: context(false), Owner: owner("Scope"), ContextParams: []string{"lazyObject.*"}},
		{Source: "src/cmd/compile/internal/types2/selection.go", WholeFile: true, Context: context(false), Owner: owner("Selection")},
		{Source: "src/cmd/compile/internal/types2/signature.go", WholeFile: true, Context: context(false, "methodExprSentinel"), Owner: owner("Signature"), ContextParams: []string{"Checker.*"}},
		{Source: "src/cmd/compile/internal/types2/sizes.go", WholeFile: true, Context: context(false, "gcArchSizes", "stdSizes"), ReadTables: []string{"basicSizes"}},
		{Source: "src/cmd/compile/internal/types2/slice.go", WholeFile: true, Context: context(false), Owner: owner("Slice")},
		{Source: "src/cmd/compile/internal/types2/stmt.go", WholeFile: true, Context: context(false), ContextParams: []string{"Checker.*"}},
		{Source: "src/cmd/compile/internal/types2/struct.go", WholeFile: true, Context: context(false), Owner: owner("Struct"), ContextParams: []string{"Checker.*"}},
		{Source: "src/cmd/compile/internal/types2/subst.go", WholeFile: true, Context: context(false), ContextParams: []string{"Checker.*", "substMap.*", "subster.*"}},
		{Source: "src/cmd/compile/internal/types2/termlist.go", WholeFile: true, Context: context(false, "allTermlist"), ContextParams: []string{"termlist.*"}},
		{Source: "src/cmd/compile/internal/types2/trie.go", WholeFile: true, Context: context(false)},
		{Source: "src/cmd/compile/internal/types2/tuple.go", WholeFile: true, Context: context(false), Owner: owner("Tuple"), Nullable: map[string][]string{"Tuple.String": {"t"}}},
		{Source: "src/cmd/compile/internal/types2/type.go", WholeFile: true, Context: context(false)},
		{Source: "src/cmd/compile/internal/types2/typelists.go", WholeFile: true, Context: context(false), Owner: owner("TypeParamList", "TypeList")},
		{Source: "src/cmd/compile/internal/types2/typeparam.go", WholeFile: true, Context: context(false, "lastID"), Owner: owner("TypeParam"), ContextParams: []string{"Checker.*"}, ContextMethods: map[string]string{"nextID": "nextTypeParamID"}},
		{Source: "src/cmd/compile/internal/types2/typeset.go", WholeFile: true, Context: context(false, "invalidTypeSet", "topTypeSet"), Owner: owner("_TypeSet")},
		{Source: "src/cmd/compile/internal/types2/typestring.go", WholeFile: true, Context: context(false), Owner: owner("typeWriter")},
		{Source: "src/cmd/compile/internal/types2/typeterm.go", WholeFile: true, Context: context(false), ContextParams: []string{"term.*"}},
		{Source: "src/cmd/compile/internal/types2/typexpr.go", WholeFile: true, Context: context(false), ContextParams: []string{"Checker.*"}},
		{Source: "src/cmd/compile/internal/types2/under.go", WholeFile: true, Context: context(false), ContextParams: []string{"typeError.*"}, ZeroValues: []string{"emptyTypeError"}},
		{Source: "src/cmd/compile/internal/types2/unify.go", WholeFile: true, Context: context(false), Owner: owner("unifier")},
		{Source: "src/cmd/compile/internal/types2/union.go", WholeFile: true, Context: context(false), Owner: owner("Union")},
		{Source: "src/cmd/compile/internal/types2/universe.go", WholeFile: true, Context: context(true, "Typ", "Universe", "Unsafe", "basicAliases", "predeclaredConsts", "predeclaredFuncs", "universeAny", "universeBool", "universeByte", "universeComparable", "universeError", "universeIota", "universeRune")},
		{Source: "src/cmd/compile/internal/types2/util.go", WholeFile: true, Context: context(false, "kind2tok")},
		{Source: "src/cmd/compile/internal/types2/validtype.go", WholeFile: true, Context: context(false), ContextParams: []string{"Checker.*"}},
		{Source: "src/cmd/compile/internal/types2/version.go", WholeFile: true, Context: context(false, "go1_13", "go1_14", "go1_17", "go1_18", "go1_20", "go1_21", "go1_22", "go1_23", "go1_26", "go1_27", "go1_9", "go_current"), ContextParams: []string{"Checker.*", "goVersion.*"}},
	}
}

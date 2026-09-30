package port

// gcFixedPointMigration selects the complete declaration fixed point in Main.
// Command setup and process services remain outside this reviewed stage boundary.
func gcFixedPointMigration(units []Unit) []Unit {
	const root = "src/cmd/compile/internal/"
	var source Unit
	for _, u := range units {
		if u.Source == root+"gc/obj.go" {
			source = u
			break
		}
	}
	ctx := *source.Context
	ctx.Fields = nil
	state := append([]StateRule{}, source.State...)
	state = append(state, StateRule{Source: root + "base/timings.go", Variable: "Timer", Field: "Package.Noder.Typecheck.IR.Base.Timer"}, StateRule{Source: root + "base/debug.go", Variable: "Debug", Field: "Package.Noder.Typecheck.IR.Debug"}, StateRule{Source: root + "ir/func.go", Variable: "CurFunc", Field: "Package.Noder.Typecheck.IR.CurFunc"})
	calls := append([]CallRule{}, source.Calls...)
	for _, name := range []string{"WriteBasicTypes", "WriteRuntimeTypes", "NeedRuntimeType"} {
		calls = append(calls, CallRule{Source: root + "reflectdata/reflect.go", Function: name, Method: "Package.Noder.ReflectData." + name})
	}
	calls = append(calls, CallRule{Source: root + "gc/compile.go", Function: "enqueueFunc", Method: "EnqueueFunc", KeepDefinition: true}, CallRule{Source: root + "gc/compile.go", Function: "compileFunctions", Method: "CompileQueued", KeepDefinition: true}, CallRule{Source: root + "gc/obj.go", Function: "dumpdata", Method: "DumpData", KeepDefinition: true})
	for _, x := range []struct{ file, name, method string }{
		{"ir/dump.go", "MatchAstDump", "Package.Noder.Typecheck.IR.MatchAstDump"},
		{"ir/dump.go", "AstDump", "Package.Noder.Typecheck.IR.AstDump"},
		{"bloop/bloop.go", "Walk", "bloop.Walk"},
		{"inline/interleaved/interleaved.go", "DevirtualizeAndInlinePackage", "Package.Noder.Interleaved.DevirtualizeAndInlinePackage"},
		{"noder/reader.go", "MakeWrappers", "Package.Noder.MakeWrappers"},
		{"loopvar/loopvar.go", "ForCapture", "loopvar.ForCapture"},
		{"loopvar/loopvar.go", "LogTransformations", "loopvar.LogTransformations"},
		{"pkginit/init.go", "MakeTask", "Package.MakeTask"},
		{"deadlocals/deadlocals.go", "Funcs", "deadlocals.Funcs"},
		{"escape/escape.go", "Funcs", "Lowering.Escape.Funcs"},
		{"slice/slice.go", "Funcs", "slice.Funcs"},
		{"ssagen/nowb.go", "EnableNoWriteBarrierRecCheck", "Backend.EnableNoWriteBarrierRecCheck"},
		{"ssagen/nowb.go", "NoWriteBarrierRecCheck", "Backend.NoWriteBarrierRecCheck"},
		{"staticinit/sched.go", "AddKeepRelocations", "Package.Noder.StaticInit.AddKeepRelocations"},
		{"ssagen/pgen.go", "CheckLargeStacks", "Backend.CheckLargeStacks"},
		{"typecheck/dcl.go", "CheckFuncStack", "Package.Noder.Typecheck.CheckFuncStack"},
	} {
		calls = append(calls, CallRule{Source: root + x.file, Function: x.name, Method: x.method})
	}
	units = append(units, Unit{Source: root + "gc/main.go", Functions: []string{"Main", "MainFrontend", "MainFinish", "MainChecks"}, Context: &ctx, State: state, Calls: calls, MethodCalls: []MethodCallRule{{Source: "src/cmd/internal/obj/sym.go", Method: "Link.NumberSyms", Adapter: "numberSymbols"}}, ContextMethods: map[string]string{"Main": "compileDeclarations", "MainFrontend": "prepareFrontend", "MainFinish": "finishPackage", "MainChecks": "checkPackage"}, FunctionRegions: map[string]FunctionRegionRule{"MainChecks": {SourceFunction: "Main", First: SourceCallAnchor{Package: "cmd/compile/internal/ssagen", Member: "CheckLargeStacks"}, Last: SourceCallAnchor{Package: "cmd/compile/internal/base", Member: "Fatalf", Enclosing: "if", StringArgument: &SourceStringArgument{Index: 0, Value: "%d uncompiled functions"}}}, "MainFinish": {SourceFunction: "Main", First: SourceCallAnchor{Package: "cmd/compile/internal/ssagen", Member: "NoWriteBarrierRecCheck", Enclosing: "if"}, Last: SourceCallAnchor{Package: "cmd/compile/internal/base", Member: "Ctxt.NumberSyms"}}, "MainFrontend": {SourceFunction: "Main", First: SourceCallAnchor{Package: "cmd/compile/internal/ir", Member: "MatchAstDump", Enclosing: "range", StringArgument: &SourceStringArgument{Index: 1, Value: "start"}}, Last: SourceCallAnchor{Package: "cmd/compile/internal/reflectdata", Member: "WriteBasicTypes"}, EndBefore: true, Inputs: []string{"symABIs", "profile"}}, "Main": {First: SourceCallAnchor{Package: "cmd/compile/internal/reflectdata", Member: "WriteBasicTypes"}, Last: SourceCallAnchor{Package: "cmd/compile/internal/base", Member: "Timer.AddEvent"}, Inputs: []string{"symABIs", "profile"}}}})
	return units
}

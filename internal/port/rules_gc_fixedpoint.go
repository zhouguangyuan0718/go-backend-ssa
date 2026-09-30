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
	state = append(state, StateRule{Source: root + "base/timings.go", Variable: "Timer", Field: "Package.Noder.Typecheck.IR.Base.Timer"}, StateRule{Source: root + "base/debug.go", Variable: "Debug", Field: "Package.Noder.Typecheck.IR.Debug"})
	calls := append([]CallRule{}, source.Calls...)
	for _, name := range []string{"WriteBasicTypes", "WriteRuntimeTypes", "NeedRuntimeType"} {
		calls = append(calls, CallRule{Source: root + "reflectdata/reflect.go", Function: name, Method: "Package.Noder.ReflectData." + name})
	}
	calls = append(calls, CallRule{Source: root + "gc/compile.go", Function: "enqueueFunc", Method: "EnqueueFunc", KeepDefinition: true}, CallRule{Source: root + "gc/compile.go", Function: "compileFunctions", Method: "CompileQueued", KeepDefinition: true})
	units = append(units, Unit{Source: root + "gc/main.go", Functions: []string{"Main"}, Context: &ctx, State: state, Calls: calls, ContextMethods: map[string]string{"Main": "compileDeclarations"}, FunctionRegions: map[string]FunctionRegionRule{"Main": {First: SourceCallAnchor{Package: "cmd/compile/internal/reflectdata", Member: "WriteBasicTypes"}, Last: SourceCallAnchor{Package: "cmd/compile/internal/base", Member: "Timer.AddEvent"}, Inputs: []string{"symABIs", "profile"}}}})
	return units
}

package port

// The phase table captures experiment policy after compiler inputs are bound.
// Metadata-only opcode contexts initialize independently. Native table entries,
// option handling, and every ordering constraint remain source-derived.
func ssaPhaseMigration(units []Unit) []Unit {
	for i := range units {
		u := &units[i]
		if u.Source != "src/cmd/compile/internal/ssa/compile.go" {
			continue
		}
		u.RangeContinuations = map[string]RangeContinuationRule{"Compile": {Range: "passes", Method: "runCompileStages", Argument: "f"}}
		u.WholeFile = true
		u.Declarations = nil
		u.ContextFrom = map[string]ContextSource{"Compile": {Argument: "f", Resolver: "diagnosticContext"}, "Func.DumpFileForPhase": {Argument: "f", Resolver: "diagnosticContext"}}
		u.HostCalls = []HostCallRule{{Package: "os", Function: "Getenv", Method: "dumpEnvironment", StringArgument: "GOSSADIR"}, {Package: "os", Function: "Create", Method: "createDump"}, {Package: "runtime", Function: "ReadMemStats", Method: "readMemStats"}, {Package: "time", Function: "Now", Method: "now"}}
		u.Context.DeferInit = false
		u.Context.Init = true
		u.Context.InitGroup = "initializePhases"
		u.Context.Fields = map[string]string{}
		for _, name := range []string{"checkEnabled", "checkRandSeed", "IntrinsicsDebug", "IntrinsicsDisable", "BuildDebug", "BuildTest", "BuildStats", "BuildDump", "GenssaDump", "passes", "passOrder"} {
			u.Context.Fields[name] = name
		}
		u.State = []StateRule{{Source: "src/internal/buildcfg/exp.go", Variable: "Experiment", Field: "Types.Experiment"}}
	}
	return units
}

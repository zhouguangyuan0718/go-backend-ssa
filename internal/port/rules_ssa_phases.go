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
		u.Declarations = []string{"pass", "pass.addDump", "pass.String", "constraint", "PhaseOption"}
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

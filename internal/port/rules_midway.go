package port

// Preserve the complete native SIMD dependency analysis and syntax rewriting.
// The checker owns synthesized objects; target and diagnostic inputs are explicit.
func midwayMigration() []Unit {
	var units []Unit
	for _, name := range []string{"analysis", "check", "deepcopy", "midway", "rewrite"} {
		u := Unit{Source: "src/cmd/compile/internal/midway/" + name + ".go", WholeFile: true,
			Context: &ContextRule{Type: "Context", Constructor: "initialize", Initialize: true, Selective: true, PreserveNilArgs: true, Adapter: "adapter_context.go"}}
		switch name {
		case "analysis", "deepcopy", "rewrite":
			owner := map[string]string{"analysis": "Analyzer", "deepcopy": "DeepCopier", "rewrite": "Rewriter"}[name]
			u.Owner = &OwnerRule{Types: []string{owner}, Context: "Context", Field: "compilation"}
			u.State = append(u.State, StateRule{Source: "src/cmd/compile/internal/base/debug.go", Variable: "Debug", Field: "Debug"})
			u.Calls = append(u.Calls, CallRule{Source: "src/cmd/compile/internal/base/print.go", Function: "Warn", Method: "warn"})
		}
		switch name {
		case "analysis":
			u.Checks = map[string]string{"NewAnalyzer": "checkAnalysis"}
		case "check":
			u.Calls = append(u.Calls, CallRule{Source: "src/cmd/compile/internal/base/print.go", Function: "Fatalf", Method: "fatalf"})
		case "deepcopy":
			u.Checks = map[string]string{"NewDeepCopier": "checkCopy"}
			u.Calls = append(u.Calls, CallRule{Source: "src/cmd/compile/internal/types2/object.go", Function: "NewVar", Method: "Checker.NewVar"}, CallRule{Source: "src/cmd/compile/internal/types2/object.go", Function: "NewFunc", Method: "Checker.NewFunc"})
		case "rewrite":
			u.Checks = map[string]string{"NewRewriter": "checkRewrite"}
			u.State = append(u.State, StateRule{Source: "src/internal/buildcfg/exp.go", Variable: "Experiment", Field: "Experiment"})
			fallthrough
		case "midway":
			u.State = append(u.State, StateRule{Source: "src/internal/buildcfg/cfg.go", Variable: "GOARCH", Field: "GOARCH"})
		}
		units = append(units, u)
	}
	return units
}

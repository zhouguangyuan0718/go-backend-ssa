package port

// archiveMigration preserves the native Unified export and package container.
func archiveMigration(units []Unit) []Unit {
	const compiler = "src/cmd/compile/internal/"
	const internal = "src/cmd/internal/"
	for i := range units {
		u := &units[i]
		switch u.Source {
		case compiler + "noder/unified.go":
			u.Functions = append(u.Functions, "writeUnifiedExport")
			u.Checks = map[string]string{"writeUnifiedExport": "checkExport"}
		case internal + "bio/buf.go":
			u.Declarations = append(u.Declarations, "Writer.Close")
			u.HostFields[0].Methods = append(u.HostFields[0].Methods, "Close")
		case "src/internal/buildcfg/exp.go":
			u.Declarations = append(u.Declarations, "ExperimentFlags.Enabled", "ExperimentFlags.String", "ExperimentFlags.All")
			u.Functions = append(u.Functions, "ParseGOEXPERIMENT")
			u.Functions = append(u.Functions, "expList")
		case "src/internal/buildcfg/cfg.go":
			u.Declarations = append(u.Declarations, "GoarmFeatures.String", "Goarm64Features.String", "gowasmFeatures", "gowasmFeatures.String")
			u.Functions = append(u.Functions, "GOGOARCH")
			u.Context = &ContextRule{Type: "Context", Constructor: "newContext", Selective: true, Adapter: "adapter_context.go"}
			for _, name := range []string{"GOARCH", "GO386", "GOAMD64", "GOARM", "GOARM64", "GOMIPS", "GOMIPS64", "GOPPC64", "GORISCV64", "GOWASM"} {
				u.State = append(u.State, StateRule{Source: u.Source, Variable: name, Field: name, Input: true})
			}
		case internal + "objabi/util.go":
			u.Functions = append(u.Functions, "HeaderString")
			u.Context = &ContextRule{Type: "Context", Constructor: "newContext", Selective: true, Adapter: "adapter_path.go"}
			for _, name := range []string{"GOOS", "GOARCH", "Version"} {
				u.State = append(u.State, StateRule{Source: "src/internal/buildcfg/cfg.go", Variable: name, Field: "ObjectConfig." + name})
			}
			for j := range u.State {
				if u.State[j].Variable == "Version" {
					u.State[j].TypeSource = "src/cmd/dist/buildruntime.go"
				}
			}
			u.State = append(u.State, StateRule{Source: "src/internal/buildcfg/exp.go", Variable: "Experiment", Field: "Experiment"})
			u.Calls = append(u.Calls, CallRule{Source: "src/internal/buildcfg/cfg.go", Function: "GOGOARCH", Method: "ObjectConfig.GOGOARCH"})
		case compiler + "gc/obj.go":
			u.Functions = append(u.Functions, "dumpobj1", "printObjHeader", "startArchiveEntry", "finishArchiveEntry", "dumpCompilerObj", "dumpLinkerObj")
			u.Declarations = append(u.Declarations, "modeCompilerObj")
			u.Context.RedirectTrace = true
			u.State = append(u.State, StateRule{Source: compiler + "typecheck/target.go", Variable: "Target", Field: "Package.Noder.Typecheck.Target"})
			u.Calls = append(u.Calls, CallRule{Source: internal + "bio/buf.go", Function: "Create", Method: "createOutput"}, CallRule{Source: internal + "objabi/util.go", Function: "HeaderString", Method: "header.HeaderString"}, CallRule{Source: compiler + "noder/export.go", Function: "WriteExports", Method: "Package.Noder.WriteExports"}, CallRule{Source: compiler + "base/print.go", Function: "FlushErrors", Method: "flushErrors"}, CallRule{Source: compiler + "base/print.go", Function: "ErrorExit", Method: "errorExit"})
		}
	}
	for _, u := range units {
		if u.Source == compiler+"gc/obj.go" {
			header := u
			header.Source = compiler + "gc/export.go"
			header.Functions = []string{"dumpasmhdr"}
			header.Declarations = nil
			units = append(units, header)
			break
		}
	}
	ctx := func() *ContextRule {
		return &ContextRule{Type: "Context", Constructor: "initContext", Initialize: true, PreserveNilArgs: true, RedirectTrace: true, Adapter: "adapter_context.go", Selective: true}
	}
	units = append(units, Unit{Source: compiler + "noder/linker.go", WholeFile: true, Context: ctx(), Owner: &OwnerRule{Types: []string{"linker"}, Context: "Context", Field: "compilation"}}, Unit{Source: compiler + "noder/export.go", WholeFile: true, Context: ctx()}, Unit{Source: internal + "archive/archive.go", Functions: []string{"FormatHeader"}, Declarations: []string{"HeaderSize"}})
	var ordered []Unit
	for _, u := range units {
		if u.sourcePackage() == "cmd/internal/archive" {
			ordered = append(ordered, u)
		}
	}
	for _, u := range units {
		if u.sourcePackage() != "cmd/internal/archive" {
			ordered = append(ordered, u)
		}
	}
	return ordered
}

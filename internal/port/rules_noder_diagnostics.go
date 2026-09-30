package port

// Preserve the native syntax report and shared HTML writer. Reports use host
// streams; the shared writer obtains its IR owner from its native buffer.
func noderDiagnosticMigration(units []Unit) []Unit {
	const root = "src/cmd/compile/internal/"
	for i := range units {
		if units[i].Source == root+"ir/dump.go" {
			units[i].Functions = append(units[i].Functions, "EscapedFileName")
		}
	}
	units = append(units, Unit{Source: root + "ir/html.go",
		Functions:           []string{"NewBufferedWriterCloser", "JS"},
		Declarations:        []string{"HTMLWriterBase", "HTMLWriterBase.Init", "HTMLWriterBase.Path", "HTMLWriterBase.CanonId", "HTMLWriterBase.Fatalf", "HTMLWriterBase.Close", "HTMLWriterBase.WritePhase", "HTMLWriterBase.flushPhases", "HTMLWriterBase.WriteMultiTitleColumn", "HTMLWriterBase.Printf", "HTMLWriterBase.Print", "BufferedWriterCloser", "BufferedWriterCloser.Write", "BufferedWriterCloser.Close", "RightArrow", "CSS", "JS1"},
		Owner:               &OwnerRule{Types: []string{"BufferedWriterCloser"}, Context: "Context", Field: "compilation"},
		Context:             &ContextRule{Type: "Context", Constructor: "initialize", Initialize: true, Selective: true, PreserveNilArgs: true, RedirectTrace: true, Adapter: "adapter_context.go", Fields: map[string]string{"safePhaseNameString": "safePhaseNameString"}},
		ContextParamExports: []string{"HTMLWriterBase.Init"},
		ContextFrom:         map[string]ContextSource{"HTMLWriterBase.Fatalf": {Argument: "w", Resolver: "htmlBaseContext"}, "HTMLWriterBase.Close": {Argument: "w", Resolver: "htmlBaseContext"}},
	})
	ctx := func() *ContextRule {
		return &ContextRule{Type: "Context", Constructor: "initContext", Initialize: true, Selective: true, PreserveNilArgs: true, RedirectTrace: true, Adapter: "adapter_context.go"}
	}
	dump := ctx()
	dump.Fields = map[string]string{"mu": "syntaxMu", "htmlWriters": "syntaxHTMLWriters", "orderedFuncs": "syntaxOrderedFuncs"}
	html := ctx()
	html.Fields = map[string]string{"nodeType": "syntaxNodeType"}
	return append(units,
		Unit{Source: root + "noder/dump.go", WholeFile: true, Context: dump},
		Unit{Source: root + "noder/html.go", WholeFile: true, Context: html,
			Owner:             &OwnerRule{Types: []string{"HTMLWriter"}, Context: "Context", Field: "compilation"},
			State:             []StateRule{{Source: root + "types2/universe.go", Variable: "Typ", Field: "Checker.Typ"}},
			HostCalls:         []HostCallRule{{Package: "os", Function: "OpenFile", Method: "openSyntaxHTML"}, {Package: "os", Function: "Getwd", Method: "syntaxWorkingDirectory"}},
			ContextParamCalls: []ContextParamCallRule{{Source: root + "ir/html.go", Method: "HTMLWriterBase.Init", Field: "Typecheck.IR", ContextType: "Context"}},
			Calls:             []CallRule{{Source: root + "ir/html.go", Function: "JS", Method: "Typecheck.IR.JS"}},
		})
}

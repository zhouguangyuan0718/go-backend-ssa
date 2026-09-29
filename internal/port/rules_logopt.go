package port

// Keep the native event collector. CLI path creation and LSP file emission are
// separate from inliner recording and are not part of this library entry point.
func logoptMigration() []Unit {
	return []Unit{{Source: "src/cmd/compile/internal/logopt/log_opts.go",
		Declarations: []string{"LoggedOpt", "logFormat", "None"},
		Functions:    []string{"NewLoggedOpt", "LogOpt", "LogOptRange", "Enabled"},
		Context:      &ContextRule{Type: "Context", Constructor: "newContext", Selective: true, Adapter: "adapter_context.go", Fields: map[string]string{"Format": "Format", "loggedOpts": "loggedOpts", "mu": "mu"}},
	}}
}

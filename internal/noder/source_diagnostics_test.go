package noder

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestSourceDiagnosticsAgainstOfficialCompiler(t *testing.T) {
	for _, test := range []struct{ path, lang, source, want string }{
		{"example/p", "go1.27", `package p;var X int="bad"`, "cannot use"},
		{"example/p", "go1.27", `package p;type T=interface{M()T}`, "invalid recursive type"},
		{"example/p", "go1.21", "//line generated.go:10\npackage p;func F(){for range 3{}}", "-lang was set to go1.21"},
		{"example/p", "go1.27", "//go:build go1.21\n//line generated.go:10\npackage p;func F(){for range 3{}}", "file declares //go:build go1.21"},
		{"internal/runtime/sys", "go1.27", `package sys;type nih struct{};func F[T any](){};var _=F[nih]`, "cannot use incomplete"},
		{"internal/runtime/sys", "go1.27", `package sys;type nih struct{};type Key map[nih]int`, "incomplete (or unallocatable) map key not allowed"},
		{"internal/runtime/sys", "go1.27", `package sys;type nih struct{};type Value map[int]nih`, "incomplete (or unallocatable) map value not allowed"},
		{"internal/runtime/sys", "go1.27", `package sys;type nih struct{};type Channel chan nih`, "chan of incomplete"},
	} {
		t.Run(test.want, func(t *testing.T) {
			c, _, _ := dependencyContext(t, nil, false)
			c.Ctxt.Pkgpath = test.path
			c.Types.LocalPkg = c.Types.NewPkg(test.path, "")
			c.Flag.Lang = test.lang
			c.Flag.ErrorURL = true
			dir := t.TempDir()
			file := filepath.Join(dir, "input.go")
			if e := os.WriteFile(file, []byte(test.source), 0600); e != nil {
				t.Fatal(e)
			}
			parsed := parseSource(t, c, file, test.source)
			_, e := c.CompileParsedPackage([]*ParsedFile{parsed})
			errors, ok := e.(SourceErrors)
			if !ok || !strings.Contains(e.Error(), test.want) {
				t.Fatalf("unexpected library result: %v", e)
			}
			cmd := exec.Command("go", "tool", "compile", "-p", test.path, "-lang", test.lang, "-errorurl", "-o", filepath.Join(dir, "out.o"), file)
			cmd.Env = append(os.Environ(), "GOWORK=off", "GOTOOLCHAIN=go1.27.1", "GOOS=linux", "GOARCH=amd64", "GOAMD64=v1", "GOEXPERIMENT=none")
			native, e := cmd.CombinedOutput()
			if e == nil {
				t.Fatal("official compiler accepted restricted source")
			}
			for _, d := range errors {
				if !strings.Contains(string(native), d.Message) {
					t.Fatalf("diagnostic differs\nport: %s\nnative: %s", d.Message, native)
				}
			}
		})
	}
}

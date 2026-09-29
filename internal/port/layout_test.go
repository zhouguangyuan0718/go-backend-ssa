package port

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPortPath(t *testing.T) {
	for source, want := range map[string]string{
		"src/cmd/compile/internal/types/pkg.go":     "internal/types/pkg.go",
		"src/cmd/compile/internal/ssa/_gen/main.go": "internal/ssa/_gen/main.go",
		"src/cmd/compile/internal/abi/abiutils.go":  "internal/abi/abiutils.go",
		"src/cmd/internal/objabi/path.go":           "internal/cmd/objabi/path.go",
		"src/internal/abi/type.go":                  "internal/go/abi/type.go",
		"src/internal/pkgbits/decoder.go":           "internal/go/pkgbits/decoder.go",
		"src/vendor/example/file.go":                "",
	} {
		if got := portPath(source); got != want {
			t.Errorf("portPath(%q) = %q, want %q", source, got, want)
		}
	}
}

func TestPlanRejectsInvalidSourcesAndAdapters(t *testing.T) {
	if err := Migration().Validate(); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name   string
		change func(*Unit)
	}{
		{"unsupported root", func(u *Unit) { u.Source = "src/vendor/p/a.go" }},
		{"source traversal", func(u *Unit) { u.Source = "src/cmd/compile/../p.go" }},
		{"unmarked adapter", func(u *Unit) { u.Context.Adapter = "context.go" }},
		{"adapter traversal", func(u *Unit) { u.Context.Adapter = "../adapter_context.go" }},
		{"test adapter", func(u *Unit) { u.Context.Adapter = "adapter_context_test.go" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := Migration()
			for i := range p.Units {
				if p.Units[i].Context != nil {
					tc.change(&p.Units[i])
					break
				}
			}
			if err := p.Validate(); err == nil {
				t.Fatal("accepted invalid migration rule")
			}
		})
	}
}

func TestMigrateLegacyOwnedFiles(t *testing.T) {
	for _, edited := range []bool{false, true} {
		t.Run(map[bool]string{false: "move", true: "preserve edit"}[edited], func(t *testing.T) {
			dir := t.TempDir()
			oldName := "internal/intern/upstream_pkg.go"
			old := renderedFiles(t, map[string]string{oldName: "original"})
			old.Ledger.Tool = "syncgo-m1-v1"
			b, err := JSON(old.Ledger)
			if err != nil {
				t.Fatal(err)
			}
			writeTestFile(t, dir, ledgerPath, string(b))
			writeTestFile(t, dir, oldName, "original")
			if edited {
				writeTestFile(t, dir, oldName, "user edit")
			}
			name := "internal/types/pkg.go"
			r := renderedFiles(t, map[string]string{name: "migrated"})
			err = Install(dir, r)
			if edited {
				if err == nil || !strings.Contains(err.Error(), "edited") {
					t.Fatalf("lost edited legacy file: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if err := Check(dir, r); err != nil {
				t.Fatal(err)
			}
			if _, err := os.Stat(filepath.Join(dir, oldName)); !os.IsNotExist(err) {
				t.Fatal("legacy copy remains")
			}
		})
	}
}

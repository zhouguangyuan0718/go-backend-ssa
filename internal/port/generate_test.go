package port

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func renderedFiles(t *testing.T, files map[string]string) *Rendered {
	t.Helper()
	r := &Rendered{Files: map[string][]byte{}, Ledger: Generated{SchemaVersion: 1, Tool: ToolVersion}}
	for _, name := range sortedKeys(files) {
		b := []byte(files[name])
		r.Files[name] = b
		r.Ledger.Files = append(r.Ledger.Files, Provenance{Output: name, OutputSHA256: hash(b)})
	}
	b, e := JSON(r.Ledger)
	if e != nil {
		t.Fatal(e)
	}
	r.Files[ledgerPath] = b
	return r
}

func TestInstallOwnershipAndStaleCleanup(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "internal/p/context.go", "handwritten")
	r := renderedFiles(t, map[string]string{"internal/p/a.go": "first", "internal/p/b.go": "stale"})
	if e := Install(dir, r); e != nil {
		t.Fatal(e)
	}
	if e := Check(dir, r); e != nil {
		t.Fatal(e)
	}
	r2 := renderedFiles(t, map[string]string{"internal/p/a.go": "second"})
	if e := Install(dir, r2); e != nil {
		t.Fatal(e)
	}
	if e := Check(dir, r2); e != nil {
		t.Fatal(e)
	}
	if _, e := os.Stat(filepath.Join(dir, "internal/p/b.go")); !os.IsNotExist(e) {
		t.Fatalf("stale generated file retained: %v", e)
	}
	b, _ := os.ReadFile(filepath.Join(dir, "internal/p/context.go"))
	if string(b) != "handwritten" {
		t.Fatal("adapter overwritten")
	}
	writeTestFile(t, dir, "internal/p/a.go", "manual edit")
	if e := Install(dir, r2); e == nil || !strings.Contains(e.Error(), "edited") {
		t.Fatalf("accepted edited generated file: %v", e)
	}
	if e := Check(dir, r2); e == nil {
		t.Fatal("missed generated drift")
	}
}

func TestInstallPreflightsAllFiles(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "internal/p/z.go", "unowned")
	r := renderedFiles(t, map[string]string{"internal/p/a.go": "new", "internal/p/z.go": "replace"})
	if e := Install(dir, r); e == nil {
		t.Fatal("overwrote unowned file")
	}
	if _, e := os.Stat(filepath.Join(dir, "internal/p/a.go")); !os.IsNotExist(e) {
		t.Fatal("partial install before conflict")
	}
	if _, e := os.Stat(filepath.Join(dir, ledgerPath)); !os.IsNotExist(e) {
		t.Fatal("ledger written on failure")
	}
}

func TestInstallRejectsSymlinkAndInvalidLedger(t *testing.T) {
	t.Run("symlink", func(t *testing.T) {
		dir := t.TempDir()
		outside := t.TempDir()
		if e := os.MkdirAll(filepath.Join(dir, "internal"), 0755); e != nil {
			t.Fatal(e)
		}
		if e := os.Symlink(outside, filepath.Join(dir, "internal/p")); e != nil {
			t.Fatal(e)
		}
		if e := Install(dir, renderedFiles(t, map[string]string{"internal/p/a.go": "new"})); e == nil {
			t.Fatal("followed symlink")
		}
		entries, _ := os.ReadDir(outside)
		if len(entries) != 0 {
			t.Fatal("wrote outside project")
		}
	})
	t.Run("ledger traversal", func(t *testing.T) {
		dir := t.TempDir()
		writeTestFile(t, dir, ledgerPath, `{"schema_version":1,"files":[{"output":"../upstream_x.go","output_sha256":"`+strings.Repeat("0", 64)+`"}]}`)
		if e := Install(dir, renderedFiles(t, map[string]string{"internal/p/a.go": "new"})); e == nil {
			t.Fatal("accepted traversal in ledger")
		}
	})
}

func TestAuditRejectsUnlistedProductionFile(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "internal/p/hidden.go", "package p; var hidden = make(map[string]int)")
	p := Plan{Units: []Unit{{Source: "src/cmd/compile/internal/p/a.go"}}}
	if e := auditProductionFiles(dir, p); e == nil || !strings.Contains(e.Error(), "unlisted production") {
		t.Fatalf("missed hidden state file: %v", e)
	}
}

func TestLedgerRejectsUnknownFields(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, ledgerPath, `{"schema_version":1,"commmit":"typo"}`)
	if _, err := readLedger(dir); err == nil {
		t.Fatal("accepted unknown ownership field")
	}
}

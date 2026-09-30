package gcintegration

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"go-backend-ssa/internal/port"
	"os"
	"path/filepath"
	"testing"
)

func TestPinnedBootstrapPatch(t *testing.T) {
	repo := os.Getenv("GO_SSA_SOURCE_REPO")
	if repo == "" {
		t.Skip("set GO_SSA_SOURCE_REPO for the pinned dist bootstrap patch")
	}
	s, err := port.OpenSource(context.Background(), repo, port.Baseline())
	if err != nil {
		t.Fatal(err)
	}
	original, err := s.Read("src/cmd/dist/buildtool.go")
	if err != nil {
		t.Fatal(err)
	}
	patched, err := patchBootstrap(original)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := patchBootstrap(patched); err == nil {
		t.Fatal("already patched source accepted")
	}
	if _, err := patchBootstrap(bytes.Replace(original, []byte("var bootstrapDirs"), []byte("var changedBootstrapDirs"), 1)); err == nil {
		t.Fatal("changed upstream declaration accepted")
	}
}

func verifyIntegrationFiles(t *testing.T, tree string, m *Manifest) {
	t.Helper()
	if len(m.IntegrationFiles) != 4 {
		t.Fatal("incomplete integration provenance", m.IntegrationFiles)
	}
	if m.VendorModule != vendorModule || len(m.VendoredFiles) != len(m.Files) || len(m.Files) == 0 {
		t.Fatal("incomplete vendored source provenance")
	}
	for name, expected := range m.VendoredFiles {
		data, err := os.ReadFile(filepath.Join(tree, "src/cmd/vendor", vendorModule, filepath.FromSlash(name)))
		if err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256(data)
		if hex.EncodeToString(sum[:]) != expected {
			t.Fatal("vendored source differs from manifest", name)
		}
	}
	for name, expected := range m.IntegrationFiles {
		data, err := os.ReadFile(filepath.Join(tree, filepath.FromSlash(name)))
		if err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256(data)
		if hex.EncodeToString(sum[:]) != expected {
			t.Fatal("integration source differs from manifest", name)
		}
	}
}

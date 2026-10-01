package gcintegration

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPinnedCodegenRegression(t *testing.T) {
	if os.Getenv("GO_SSA_CODEGEN_REGRESSION") == "" {
		t.Skip("set GO_SSA_CODEGEN_REGRESSION and Linux driver fixture inputs")
	}
	testPinnedTestDirectory(t, "codegen", "^Test/codegen/")
}
func TestPinnedLanguageRegression(t *testing.T) {
	if os.Getenv("GO_SSA_LANGUAGE_REGRESSION") == "" {
		t.Skip("set GO_SSA_LANGUAGE_REGRESSION and Linux driver fixture inputs")
	}
	testPinnedTestDirectory(t, "language", "^Test$")
}
func testPinnedTestDirectory(t *testing.T, suite, run string) {
	arch := os.Getenv("GO_SSA_TEST_ARCH")
	if arch == "" {
		arch = "arm64"
	}
	f := newLinuxDriverFixture(t, arch)
	// Release SDKs omit GOROOT/test, which silently skips this entire suite.
	// Make an owned reference SDK and add the exact pinned tests from the source
	// snapshot; the supplied SDK and the original source checkout remain inputs.
	reference := t.TempDir()
	if err := os.CopyFS(reference, os.DirFS(f.bootstrap)); err != nil {
		t.Fatal(err)
	}
	testRoot := filepath.Join(reference, "test")
	if err := os.RemoveAll(testRoot); err != nil {
		t.Fatal(err)
	}
	if err := os.CopyFS(testRoot, os.DirFS(filepath.Join(f.tree, "test"))); err != nil {
		t.Fatal(err)
	}
	count := 0
	if err := filepath.WalkDir(testRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.HasSuffix(path, ".go") {
			count++
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if count < 100 {
		t.Fatalf("pinned Go test directory is incomplete: %d source files", count)
	}
	t.Logf("official reference SDK includes %d pinned Go test source files", count)
	f.bootstrap = reference
	// Native run tests execute from GOROOT/test and may create linker output.
	f.writableReference = true
	// This package consists solely of external tests, so its compiler package
	// identity is testdir_test. Require direct test-source compiles as well.
	testPinnedNativeRegressionFixture(t, f, suite, run, []string{"cmd/internal/testdir_test", "command-line-arguments"}, "cmd/internal/testdir")
}

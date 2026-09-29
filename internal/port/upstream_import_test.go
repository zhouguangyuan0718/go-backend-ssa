package port

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// Produce finalized export data with the fixed official compiler. Only tests
// invoke this process; both readers consume exactly the same untouched payload.
func prepareNativeImport(t *testing.T, root, dir string, newInliner bool) {
	t.Helper()
	source, err := os.ReadFile(filepath.Join(root, "internal/port/testdata/noder_import_fixture.go.txt"))
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(dir, "import-fixture", "dep.go")
	archive := filepath.Join(dir, "dep.a")
	writeTestFile(t, dir, "import-fixture/dep.go", string(source))
	experiment := ""
	if newInliner {
		experiment = "newinliner"
	}
	run := func(args ...string) []byte {
		t.Helper()
		cmd := exec.Command("go", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GOWORK=off", "GOTOOLCHAIN=go1.27.1", "GOOS=linux", "GOARCH=amd64", "GOEXPERIMENT="+experiment)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("native export %v: %v\n%s", args, err, out)
		}
		return out
	}
	run("tool", "compile", "-p", "example/dep", "-pack", "-o", archive, file)
	data := run("tool", "pack", "p", archive, "__.PKGDEF")
	start, end := bytes.Index(data, []byte("\n$$B\nu")), bytes.LastIndex(data, []byte("\n$$\n"))
	if start < 0 || end <= start {
		t.Fatal("native unified export payload missing")
	}
	writeTestFile(t, dir, "reader-import", string(data[start+len("\n$$B\nu"):end]))
}

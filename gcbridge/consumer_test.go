package gcbridge_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
)

func TestIndependentConsumerModule(t *testing.T) {
	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	mod := "module consumer\n\ngo 1.27.1\n\nrequire go-backend-ssa v0.0.0\nreplace go-backend-ssa => " + strconv.Quote(root) + "\n"
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(mod), 0644); err != nil {
		t.Fatal(err)
	}
	source, err := os.ReadFile(filepath.Join(root, "testdata/consumer/consumer_test.go"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "consumer_test.go"), source, 0644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("go", "test", "-race", ".")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOWORK=off", "GOTOOLCHAIN=go1.27.1")
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("external module failed: %v\n%s", err, b)
	}
}

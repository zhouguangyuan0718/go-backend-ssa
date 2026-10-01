package gcintegration

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// Native package tests exercise programs built by the replacement driver. The
// same target SDK and container run the official control first.
func TestPinnedRuntimeRegression(t *testing.T) {
	if os.Getenv("GO_SSA_RUNTIME_REGRESSION") == "" {
		t.Skip("set GO_SSA_RUNTIME_REGRESSION and Linux driver fixture inputs")
	}
	testPinnedNativeRegression(t, "runtime", "internal/abi", "internal/runtime/atomic", "sync", "sync/atomic", "reflect", "runtime")
}

func testPinnedNativeRegression(t *testing.T, suite string, packages ...string) {
	arch := os.Getenv("GO_SSA_TEST_ARCH")
	if arch == "" {
		arch = "arm64"
	}
	f := newLinuxDriverFixture(t, arch)
	for _, name := range []string{"addr2line", "cover", "nm", "objdump", "pack", "test2json", "vet"} {
		relative := "pkg/tool/linux_" + arch + "/" + name
		data, err := os.ReadFile(filepath.Join(f.bootstrap, relative))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(f.tree, relative), data, 0755); err != nil {
			t.Fatal(err)
		}
	}
	for _, library := range []bool{false, true} {
		t.Run(fmt.Sprint(library), func(t *testing.T) {
			name := fmt.Sprintf("%s-regression-%t", suite, library)
			args := append([]string{"test", "-json", "-short", "-count=1", "-p=4", "-timeout=15m"}, packages...)
			out := f.goRun(t, library, name, args...)
			passed, skipped := 0, 0
			for _, line := range bytes.Split(out, []byte{'\n'}) {
				var event struct{ Action, Test string }
				if json.Unmarshal(line, &event) != nil {
					continue
				}
				if event.Test != "" {
					if event.Action == "pass" {
						passed++
					}
					if event.Action == "skip" {
						skipped++
					}
				}
			}
			t.Logf("native %s suite: %d passed tests, %d skipped tests", suite, passed, skipped)
			if library {
				f.verifyTrace(t, name, packages...)
			}
		})
	}
}

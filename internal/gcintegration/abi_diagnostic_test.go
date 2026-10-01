package gcintegration

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestPinnedABIDiagnostic(t *testing.T) {
	if os.Getenv("GO_SSA_RUNTIME_REGRESSION") == "" {
		t.Skip("set GO_SSA_RUNTIME_REGRESSION and Linux driver fixture inputs")
	}
	arch := os.Getenv("GO_SSA_TEST_ARCH")
	if arch == "" {
		arch = "arm64"
	}
	f := newLinuxDriverFixture(t, arch)
	for _, name := range []string{"x.go", "x.s"} {
		data, err := os.ReadFile(filepath.Join(f.bootstrap, "src/internal/abi/testdata", name))
		if err != nil {
			t.Fatal(err)
		}
		f.write(t, name, string(data))
	}
	var reference []byte
	for _, library := range []bool{false, true} {
		t.Run(fmt.Sprint(library), func(t *testing.T) {
			name := fmt.Sprintf("abi-diagnostic-%t", library)
			config := f.goRun(t, library, name, "list", "-deps", "-export", "-f", `{{if .Export}}packagefile {{.ImportPath}}={{.Export}}{{end}}`, "internal/abi")
			f.write(t, name+".importcfg", string(config))
			f.goRun(t, library, name, "tool", "asm", "-p=p", "-gensymabis", "-o", "/work/"+name+".symabi", "x.s")
			root := "/bootstrap"
			if library {
				root = "/goroot"
			}
			env := []string{"GOROOT=" + root, "GOTOOLCHAIN=local", "GOWORK=off", "GOENV=off", "GOOS=linux", "GOARCH=" + arch, "GOAMD64=v1", "GOARM64=v8.0", "GOEXPERIMENT="}
			f.write(t, name+".o", "stale output")
			out, err := f.commandResult(env, root+"/pkg/tool/linux_"+arch+"/compile", "-importcfg=/work/"+name+".importcfg", "-p=p", "-symabis", "/work/"+name+".symabi", "-o", "/work/"+name+".o", "x.go")
			if err == nil {
				t.Fatal("ABI mismatch was accepted")
			}
			exit, ok := err.(*exec.ExitError)
			if !ok || exit.ExitCode() != 2 {
				t.Errorf("source error exit = %v, want 2", err)
			}
			if _, statErr := os.Stat(filepath.Join(f.work, name+".o")); !os.IsNotExist(statErr) {
				t.Errorf("stale compiler output retained: %v", statErr)
			}
			t.Logf("ABI diagnostics:\n%s", out)
			if !library {
				reference = out
			} else if !bytes.Equal(out, reference) {
				t.Fatalf("ABI diagnostics differ from official compiler:\n%s", out)
			}
		})
	}
}

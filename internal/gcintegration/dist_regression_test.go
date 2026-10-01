package gcintegration

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Exercise the pinned dist runner's target-specific build/link variants in
// addition to the full language/compiler/runtime suites. Both lanes use the
// same SDK, C compiler and machine. Original inputs remain read-only.
func TestPinnedDistRegression(t *testing.T) {
	if os.Getenv("GO_SSA_DIST_REGRESSION") == "" {
		t.Skip("set GO_SSA_DIST_REGRESSION and Linux driver fixture inputs")
	}
	arch := os.Getenv("GO_SSA_TEST_ARCH")
	if arch == "" {
		arch = "arm64"
	}
	f := newLinuxDriverFixture(t, arch)
	reference := t.TempDir()
	if err := os.CopyFS(reference, os.DirFS(f.bootstrap)); err != nil {
		t.Fatal(err)
	}
	f.bootstrap = reference
	f.writableReference = true
	toolRel := "pkg/tool/linux_" + arch
	tools, err := os.ReadDir(filepath.Join(reference, toolRel))
	if err != nil {
		t.Fatal(err)
	}
	for _, tool := range tools {
		if tool.IsDir() || tool.Name() == "compile" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(reference, toolRel, tool.Name()))
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(filepath.Join(f.tree, toolRel, tool.Name()), data, 0755); err != nil {
			t.Fatal(err)
		}
	}
	f.goRun(t, false, "dist-build", "build", "-o", "/work/dist", "cmd/dist")
	names := []string{"reflect:pie_internal", "crypto/internal/fips140test:pie_internal", "os/user:pie_internal", "crypto/internal/fips140test:exe_external", "crypto/internal/fips140test:pie_external", "sync:cpu10", "cmd/cgo/internal/test", "cmd/cgo/internal/teststdio", "cmd/cgo/internal/testtls", "runtime/debug", "runtime/pprof"}
	if arch == "amd64" {
		names = append(names, "reflect:spectre")
	}
	for _, library := range []bool{false, true} {
		t.Run(fmt.Sprint(library), func(t *testing.T) {
			root := "/bootstrap"
			if library {
				root = "/goroot"
			}
			name := fmt.Sprintf("dist-regression-%t", library)
			env := []string{"GOROOT=" + root, "GOCACHE=/work/cache" + root, "GOTOOLCHAIN=local", "GOWORK=off", "GOFLAGS=", "GOENV=off", "GOOS=linux", "GOARCH=" + arch, "GOAMD64=v1", "GOARM64=v8.0", "GOEXPERIMENT=", "CGO_ENABLED=1", "CC=gcc", "GOPROXY=off", "GOSUMDB=off", "GOMAXPROCS=4", "GO_TEST_SHORT=1", "GO_BUILDER_NAME=go-backend-ssa-local", "GO_SSA_BRIDGE_TRACE=/work/" + name + ".jsonl"}
			// The fixture supplies all host tools and a freshly built replacement
			// compiler. Builder mode suppresses unrelated implicit tool reinstalls.
			args := append([]string{"test", "-no-rebuild", "-v"}, names...)
			out := f.command(t, env, "/work/dist", args...)
			if !bytes.Contains(out, []byte("ALL TESTS PASSED")) {
				t.Fatalf("dist completion missing:\n%s", out)
			}
			for _, pkg := range []string{"reflect", "sync", "cmd/cgo/internal/test", "runtime/debug", "runtime/pprof"} {
				if !strings.Contains(string(out), "ok  \t"+pkg) && !strings.Contains(string(out), "ok\t"+pkg) {
					t.Fatalf("dist did not execute %s:\n%s", pkg, out)
				}
			}
			t.Logf("native dist: %d selected suites completed\n%s", len(names), out)
			if library {
				f.verifyTrace(t, name, "reflect", "sync", "runtime/debug", "runtime/pprof", "cmd/cgo/internal/test")
			}
		})
	}
}

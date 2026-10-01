package gcintegration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// Build with the actual Linux C compiler and matching native Go tools, then
// execute calls in both directions across the cgo boundary.
func TestPinnedCGODriver(t *testing.T) {
	if os.Getenv("GO_SSA_CGO_INTEGRATION") == "" {
		t.Skip("set GO_SSA_CGO_INTEGRATION, GO_SSA_SOURCE_REPO, GO_SSA_LINUX_BOOTSTRAP and target image")
	}
	arch := os.Getenv("GO_SSA_TEST_ARCH")
	if arch == "" {
		arch = "arm64"
	}
	f := newLinuxDriverFixture(t, arch)
	f.write(t, "go.mod", "module example/cgoprobe\n\ngo 1.27\n")
	f.write(t, "main.go", `package main
/*
#include <stdlib.h>
extern int GoDouble(int);
static int call_go(int x) { return GoDouble(x) + 1; }
static int sum(int a, int b) { return a+b; }
*/
import "C"
import("fmt";"runtime";"unsafe")
//export GoDouble
func GoDouble(x C.int) C.int { ch:=make(chan C.int);go func(){runtime.GC();ch<-x*2}();return <-ch }
func main(){a:=C.sum(20,22);b:=C.call_go(10);s:=C.CString("cgo");defer C.free(unsafe.Pointer(s));runtime.GC();fmt.Println(int(a),int(b),C.GoString(s))}
`)
	for _, mode := range []string{"auto", "internal", "external"} {
		t.Run(mode, func(t *testing.T) {
			var expected []byte
			for _, library := range []bool{false, true} {
				name := fmt.Sprintf("%s-%t", mode, library)
				args := []string{"build", "-a", "-p=4", "-o", "/work/" + name}
				if mode != "auto" {
					args = append(args, "-ldflags=-linkmode="+mode)
				}
				f.goRun(t, library, name, append(args, ".")...)
				actual := f.execute(t, "/work/"+name)
				if string(actual) != "42 21 cgo\n" {
					t.Fatalf("unexpected cgo execution: %q", actual)
				}
				if !library {
					expected = actual
				} else {
					if !bytes.Equal(actual, expected) {
						t.Fatalf("library execution %q differs from official %q", actual, expected)
					}
					f.verifyTrace(t, name, "runtime", "runtime/cgo", "main")
				}
			}
		})
	}
	if os.Getenv("GO_SSA_DRIVER_FULL") != "" {
		t.Run("standard", func(t *testing.T) {
			// Establish the same-version C toolchain baseline before attributing
			// any whole-distribution build failure to the replacement compiler.
			f.goRun(t, false, "official-standard", "build", "-a", "-p=4", "std", "cmd")
			f.goRun(t, true, "library-standard", "build", "-a", "-p=4", "std", "cmd")
			data := f.goRun(t, true, "standard-list", "list", "-deps", "-json", "std", "cmd")
			decoder := json.NewDecoder(bytes.NewReader(data))
			required := map[string]bool{}
			var mainFiles []string
			for decoder.More() {
				var pkg struct {
					ImportPath, Name, Dir string
					GoFiles, CgoFiles     []string
				}
				if err := decoder.Decode(&pkg); err != nil {
					t.Fatal(err)
				}
				// go list decorates PGO dependency variants with the consuming
				// main package, including the compiler-provided unsafe package.
				pkg.ImportPath, _, _ = strings.Cut(pkg.ImportPath, " [")
				if pkg.ImportPath != "unsafe" && len(pkg.GoFiles)+len(pkg.CgoFiles) != 0 {
					if pkg.Name == "main" {
						mainFiles = append(mainFiles, filepath.Join(pkg.Dir, pkg.GoFiles[0]))
						required["main"] = true
					} else {
						required[pkg.ImportPath] = true
					}
				}
			}
			var paths []string
			for path := range required {
				paths = append(paths, path)
			}
			packages := f.verifyTrace(t, "library-standard", paths...)
			if len(packages) != len(required) {
				t.Fatalf("compiled %d packages, expected %d", len(packages), len(required))
			}
			trace, err := os.ReadFile(filepath.Join(f.work, "library-standard.jsonl"))
			if err != nil {
				t.Fatal(err)
			}
			compiledFiles := map[string]bool{}
			for _, line := range bytes.Split(bytes.TrimSpace(trace), []byte{'\n'}) {
				var call struct {
					Package string
					Files   []string
				}
				if err := json.Unmarshal(line, &call); err != nil {
					t.Fatal(err)
				}
				if call.Package == "main" {
					for _, file := range call.Files {
						compiledFiles[file] = true
					}
				}
			}
			for _, file := range mainFiles {
				if !compiledFiles[file] {
					t.Fatalf("tool source did not compile through library: %s", file)
				}
			}
			t.Logf("all %d selected package identities and %d command entry packages compiled with cgo enabled", len(required), len(mainFiles))
		})
	}
}

type linuxDriverFixture struct {
	native bool
	// writableReference is set only for an owned SDK copy, never an input SDK.
	writableReference                  bool
	arch, image, bootstrap, tree, work string
	manifest                           *Manifest
}

func newLinuxDriverFixture(t *testing.T, arch string) *linuxDriverFixture {
	t.Helper()
	if arch != "amd64" && arch != "arm64" {
		t.Fatal("unsupported integration architecture", arch)
	}
	f := &linuxDriverFixture{arch: arch, image: os.Getenv("GO_SSA_LINUX_" + strings.ToUpper(arch) + "_IMAGE"), bootstrap: os.Getenv("GO_SSA_LINUX_BOOTSTRAP")}
	f.native = runtime.GOOS == "linux" && runtime.GOARCH == arch && f.image == ""
	repo := os.Getenv("GO_SSA_SOURCE_REPO")
	if repo == "" || !f.native && f.image == "" || f.bootstrap == "" {
		t.Fatal("requires source repository, matching official bootstrap SDK and a target image when not running natively")
	}
	var err error
	f.bootstrap, err = filepath.Abs(f.bootstrap)
	if err != nil {
		t.Fatal(err)
	}
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	f.tree, f.work = filepath.Join(dir, "go"), filepath.Join(dir, "work")
	if err := os.Mkdir(f.work, 0755); err != nil {
		t.Fatal(err)
	}
	f.manifest, err = Prepare(context.Background(), repo, root, f.tree, runtime.GOROOT())
	if err != nil {
		t.Fatal(err)
	}
	verifyIntegrationFiles(t, f.tree, f.manifest)
	toolDir := filepath.Join(f.tree, "pkg/tool/linux_"+arch)
	if err := os.MkdirAll(toolDir, 0755); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(filepath.Join(runtime.GOROOT(), "bin/go"), "build", "-p=4", "-pgo=off", "-o", filepath.Join(toolDir, "compile"), "cmd/compile")
	cmd.Dir = filepath.Join(f.tree, "src/cmd")
	cmd.Env = append(os.Environ(), "GOROOT="+f.tree, "GOCACHE="+filepath.Join(dir, "host-cache"), "GOTOOLCHAIN=local", "GOWORK=off", "GOFLAGS=", "GOOS=linux", "GOARCH="+arch, "GOAMD64=v1", "GOARM64=v8.0", "GOEXPERIMENT=", "CGO_ENABLED=0", "GOPROXY=off", "GOSUMDB=off", "GOMAXPROCS=4")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build target compiler: %v\n%s", err, out)
	}
	for _, name := range []string{"bin/go", "pkg/tool/linux_" + arch + "/asm", "pkg/tool/linux_" + arch + "/link", "pkg/tool/linux_" + arch + "/cgo", "pkg/tool/linux_" + arch + "/preprofile"} {
		data, err := os.ReadFile(filepath.Join(f.bootstrap, name))
		if err != nil {
			t.Fatal(err)
		}
		output := filepath.Join(f.tree, name)
		if err := os.WriteFile(output, data, 0755); err != nil {
			t.Fatal(err)
		}
	}
	if actual := string(f.goRun(t, false, "identity", "version")); actual != "go version go1.27.1 linux/"+arch+"\n" {
		t.Fatal("unexpected official compiler", actual)
	}
	if actual := f.goRun(t, true, "identity", "tool", "compile", "-V=full"); !bytes.Contains(actual, []byte(f.manifest.Library)) {
		t.Fatalf("replacement identity mismatch: %s", actual)
	}
	return f
}

func (f *linuxDriverFixture) command(t *testing.T, environment []string, executable string, args ...string) []byte {
	t.Helper()
	out, err := f.commandResult(environment, executable, args...)
	if err != nil {
		t.Fatalf("%s %v: %v\n%s", executable, args, err, out)
	}
	return out
}

func (f *linuxDriverFixture) commandResult(environment []string, executable string, args ...string) ([]byte, error) {
	if f.native {
		nativeArgs := make([]string, len(args))
		for i, arg := range args {
			nativeArgs[i] = f.nativePath(arg)
		}
		nativeEnv := append([]string{}, os.Environ()...)
		for _, entry := range environment {
			nativeEnv = append(nativeEnv, f.nativePath(entry))
		}
		cmd := exec.Command(f.nativePath(executable), nativeArgs...)
		cmd.Env = nativeEnv
		cmd.Dir = f.work
		return cmd.CombinedOutput()
	}

	referenceMount := "type=bind,source=" + f.bootstrap + ",target=/bootstrap"
	if !f.writableReference {
		referenceMount += ",readonly"
	}
	docker := []string{"run", "--rm", "--pull=never", "--platform", "linux/" + f.arch, "--entrypoint", "", "--network", "none", "--mount", referenceMount, "--mount", "type=bind,source=" + f.tree + ",target=/goroot", "--mount", "type=bind,source=" + f.work + ",target=/work", "-w", "/work"}
	for _, e := range environment {
		docker = append(docker, "-e", e)
	}
	docker = append(docker, f.image, executable)
	docker = append(docker, args...)
	return exec.Command("docker", docker...).CombinedOutput()
}

func (f *linuxDriverFixture) goRun(t *testing.T, library bool, name string, args ...string) []byte {
	t.Helper()
	root := "/bootstrap"
	if library {
		root = "/goroot"
	}
	env := []string{"GOROOT=" + root, "GOCACHE=/work/cache" + root, "GOTOOLCHAIN=local", "GOWORK=off", "GOFLAGS=", "GOENV=off", "GOOS=linux", "GOARCH=" + f.arch, "GOAMD64=v1", "GOARM64=v8.0", "GOEXPERIMENT=", "CGO_ENABLED=1", "CC=gcc", "GOPROXY=off", "GOSUMDB=off", "GOMAXPROCS=4", "GO_SSA_BRIDGE_TRACE=/work/" + name + ".jsonl"}
	return f.command(t, env, root+"/bin/go", args...)
}

func (f *linuxDriverFixture) execute(t *testing.T, executable string) []byte {
	t.Helper()
	return f.command(t, nil, executable)
}

func (f *linuxDriverFixture) write(t *testing.T, name, contents string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(f.work, name), []byte(contents), 0600); err != nil {
		t.Fatal(err)
	}
}

func (f *linuxDriverFixture) verifyTrace(t *testing.T, name string, required ...string) map[string]bool {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(f.work, name+".jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	packages := map[string]bool{}
	for _, line := range bytes.Split(bytes.TrimSpace(data), []byte{'\n'}) {
		var call struct{ Package, Library, Upstream string }
		if err := json.Unmarshal(line, &call); err != nil {
			t.Fatal(err)
		}
		if call.Library != f.manifest.Library || call.Upstream != f.manifest.Upstream {
			t.Fatal("unexpected compile provenance", call)
		}
		packages[call.Package] = true
	}
	for _, name := range required {
		if !packages[name] {
			t.Fatalf("no library compile evidence for %s", name)
		}
	}
	t.Logf("%s: %d packages compiled through library", name, len(packages))
	return packages
}

// Replace only a virtual root prefix, including values in flag/environment
// assignments. Cache suffixes such as /work/cache/bootstrap remain suffixes.
func (f *linuxDriverFixture) nativePath(value string) string {
	prefix, path, assigned := strings.Cut(value, "=")
	if assigned {
		return prefix + "=" + f.nativePath(path)
	}
	for _, root := range [][2]string{{"/bootstrap", f.bootstrap}, {"/goroot", f.tree}, {"/work", f.work}} {
		if value == root[0] || strings.HasPrefix(value, root[0]+"/") {
			return root[1] + strings.TrimPrefix(value, root[0])
		}
	}
	return value
}

func TestNativeFixturePaths(t *testing.T) {
	f := linuxDriverFixture{bootstrap: "/tmp/reference", tree: "/tmp/tree", work: "/tmp/probe"}
	for input, want := range map[string]string{"/bootstrap/bin/go": "/tmp/reference/bin/go", "GOROOT=/goroot": "GOROOT=/tmp/tree", "GOCACHE=/work/cache/bootstrap": "GOCACHE=/tmp/probe/cache/bootstrap", "-importcfg=/work/input": "-importcfg=/tmp/probe/input", "GORACE=halt_on_error=1 exitcode=66": "GORACE=halt_on_error=1 exitcode=66", "/workspace/file": "/workspace/file"} {
		if got := f.nativePath(input); got != want {
			t.Errorf("%q => %q, want %q", input, got, want)
		}
	}
}

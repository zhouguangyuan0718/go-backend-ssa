package gcintegration

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// This gate runs the actual pinned make.bash/dist bootstrap through all native
// stages, including default PGO and the final standard library/tool rebuild.
func TestPinnedDistBootstrap(t *testing.T) {
	repo := os.Getenv("GO_SSA_SOURCE_REPO")
	if repo == "" || os.Getenv("GO_SSA_DIST_BOOTSTRAP") == "" {
		t.Skip("set GO_SSA_SOURCE_REPO and GO_SSA_DIST_BOOTSTRAP")
	}
	linux := runtime.GOOS == "linux" && runtime.GOARCH == "amd64"
	image := os.Getenv("GO_SSA_LINUX_AMD64_IMAGE")
	bootstrap := os.Getenv("GO_SSA_LINUX_BOOTSTRAP")
	if linux && bootstrap == "" {
		bootstrap = runtime.GOROOT()
	}
	if !linux && (image == "" || bootstrap == "") {
		t.Fatal("set GO_SSA_LINUX_AMD64_IMAGE and GO_SSA_LINUX_BOOTSTRAP")
	}
	var err error
	bootstrap, err = filepath.Abs(bootstrap)
	if err != nil {
		t.Fatal(err)
	}
	version, err := os.ReadFile(filepath.Join(bootstrap, "VERSION"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.SplitN(string(version), "\n", 2)[0] != "go1.27.1" {
		t.Fatal("requires explicit Go 1.27.1 Linux/amd64 bootstrap")
	}
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	tree := filepath.Join(t.TempDir(), "go")
	m, err := Prepare(context.Background(), repo, root, tree, runtime.GOROOT())
	if err != nil {
		t.Fatal(err)
	}
	verifyIntegrationFiles(t, tree, m)
	targetTree, targetBootstrap := tree, bootstrap
	if !linux {
		targetTree, targetBootstrap = "/goroot", "/bootstrap"
	}
	target := func(parts ...string) string { return filepath.Join(append([]string{targetTree}, parts...)...) }
	common := []string{
		"GOROOT_BOOTSTRAP=" + targetBootstrap, "GOTOOLCHAIN=local", "GOOS=linux", "GOARCH=amd64", "GOAMD64=v1", "GOEXPERIMENT=", "CGO_ENABLED=0", "GOMAXPROCS=4", "GOPROXY=off", "GOSUMDB=off", "GOWORK=off", "GOFLAGS=", "GOENV=off", "GOCOMPILEDEBUG=", "GOSSAFUNC=", "GOSSADIR=", "GO_DISTFLAGS=-d", "BOOT_GO_GCFLAGS=", "GO_GCFLAGS=", "GO_LDFLAGS=", "BOOT_GO_LDFLAGS=",
		"GO_SSA_BRIDGE_TRACE=" + target("bootstrap-calls.jsonl"), "GOBUILDTIMELOGFILE=" + target("bootstrap-times.log"),
	}
	run := func(goroot, executable string, args ...string) []byte {
		t.Helper()
		environment := append(append([]string{}, common...), "GOROOT="+goroot)
		var cmd *exec.Cmd
		if linux {
			cmd = exec.Command(executable, args...)
			cmd.Env = append(os.Environ(), environment...)
			cmd.Dir = filepath.Join(tree, "src")
		} else {
			docker := []string{"run", "--rm", "--pull=never", "--platform", "linux/amd64", "--network", "none", "--mount", "type=bind,source=" + bootstrap + ",target=/bootstrap,readonly", "--mount", "type=bind,source=" + tree + ",target=/goroot", "-w", "/goroot/src"}
			for _, e := range environment {
				docker = append(docker, "-e", e)
			}
			docker = append(docker, image, executable)
			docker = append(docker, args...)
			cmd = exec.Command("docker", docker...)
		}
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("%s %v: %v\n%s", executable, args, err, output)
		}
		return output
	}
	if got := string(run(targetBootstrap, filepath.Join(targetBootstrap, "bin/go"), "version")); got != "go version go1.27.1 linux/amd64\n" {
		t.Fatal("unexpected bootstrap", got)
	}
	output := run(targetTree, "bash", "./make.bash")
	t.Logf("native dist bootstrap:\n%s", output)
	for _, phase := range []string{"Building Go toolchain1", "Building Go bootstrap cmd/go", "Building Go toolchain2", "Building Go toolchain3", "Building packages and commands"} {
		if !bytes.Contains(output, []byte(phase)) {
			t.Fatal("missing native bootstrap phase", phase)
		}
	}
	calls, err := os.ReadFile(filepath.Join(tree, "bootstrap-calls.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	packages := map[string]bool{}
	compilerCold, compilerPGO := false, false
	for _, line := range bytes.Split(bytes.TrimSpace(calls), []byte{'\n'}) {
		var call struct {
			Package, Library, Upstream, Profile string
			Files                               []string
		}
		if err := json.Unmarshal(line, &call); err != nil {
			t.Fatal(err)
		}
		if call.Library != m.Library || call.Upstream != m.Upstream {
			t.Fatal("unexpected bootstrap compiler provenance", call)
		}
		packages[call.Package] = true
		for _, file := range call.Files {
			if file == target("src/cmd/compile/main.go") {
				if call.Profile == "" {
					compilerCold = true
				} else {
					compilerPGO = true
				}
			}
		}
	}
	if !packages["runtime"] || !packages["fmt"] || !compilerCold || !compilerPGO {
		t.Fatalf("incomplete library bootstrap evidence: runtime=%t fmt=%t compiler cold=%t PGO=%t", packages["runtime"], packages["fmt"], compilerCold, compilerPGO)
	}
	// -d preserves the real stage compilers. Run every one and compare complete
	// archives with a matching official compiler, including the final install.
	source := []byte(`package p
 type E int
 func(E)Error()string{return "error"}
 func Err()error{return E(2)}
 func F(x int)int{var fs []func()int;for i:=range x{fs=append(fs,func()int{return i})};var n int;for _,f:=range fs{n+=f()};return n}
 `)
	if err := os.WriteFile(filepath.Join(tree, "stage.go"), source, 0600); err != nil {
		t.Fatal(err)
	}
	var baseline []byte
	stages := []string{"official", "compile1", "compile2", "compile3", "compile4", "compile"}
	for _, stage := range stages {
		executable := target("pkg/tool/linux_amd64", stage)
		goroot := targetTree
		if stage == "official" {
			executable = filepath.Join(targetBootstrap, "pkg/tool/linux_amd64/compile")
			goroot = targetBootstrap
		} else {
			if identity := run(goroot, executable, "-V=full"); !bytes.Contains(identity, []byte(m.Library)) {
				t.Fatalf("%s lacks library identity: %s", stage, identity)
			}
		}
		archive := stage + ".a"
		run(goroot, executable, "-p", "p", "-trimpath", targetTree+"=>/source", "-o", target(archive), target("stage.go"))
		data, err := os.ReadFile(filepath.Join(tree, archive))
		if err != nil {
			t.Fatal(err)
		}
		if baseline == nil {
			baseline = data
		} else if !bytes.Equal(baseline, data) {
			t.Fatalf("%s archive differs from official compiler: %d vs %d", stage, len(data), len(baseline))
		}
	}
	t.Logf("all preserved bootstrap stages and final compiler match the official %d-byte archive", len(baseline))
	// Force another compiler rebuild with the release flags used by dist.
	// The dotted alias retains canonical cmd/vendor paths inside cmd as well.
	// Force a fresh build; comparing cached compiler copies would be insufficient.
	run(targetTree, target("bin/go"), "-C", target("src/cmd"), "build", "-a", "-p=4", "-trimpath", "-ldflags=-w", "-gcflags=cmd/...=-dwarf=false", "-o", target("compile.again"), "cmd/compile")
	stage3, err := os.ReadFile(filepath.Join(tree, "pkg/tool/linux_amd64/compile3"))
	if err != nil {
		t.Fatal(err)
	}
	again, err := os.ReadFile(filepath.Join(tree, "compile.again"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(stage3, again) {
		t.Fatalf("forced compiler rebuild is not stable: stage3=%x again=%x", sha256.Sum256(stage3), sha256.Sum256(again))
	}
	t.Logf("forced compiler rebuild matches all %d bytes of toolchain3", len(again))
	// The rebuilt cmd/go, compiler, linker and runtime must work together.
	probe := []byte("package main\nimport(\"fmt\";\"reflect\")\nfunc main(){ch:=make(chan int);go func(){ch<-21*2}();fmt.Println(<-ch,reflect.DeepEqual([]int{1,2},[]int{1,2}))}\n")
	if err := os.WriteFile(filepath.Join(tree, "probe.go"), probe, 0600); err != nil {
		t.Fatal(err)
	}
	run(targetTree, target("bin/go"), "build", "-a", "-p=4", "-o", target("probe"), target("probe.go"))
	ours := run(targetTree, target("probe"))
	run(targetBootstrap, filepath.Join(targetBootstrap, "bin/go"), "build", "-p=4", "-o", target("official-probe"), target("probe.go"))
	reference := run(targetBootstrap, target("official-probe"))
	if !bytes.Equal(ours, reference) || string(ours) != "42 true\n" {
		t.Fatalf("bootstrapped program differs: %q vs %q", ours, reference)
	}
	t.Logf("%d distinct packages recorded through the library; rebuilt toolchain program matches official execution", len(packages))
}

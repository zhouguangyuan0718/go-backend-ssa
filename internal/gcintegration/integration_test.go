package gcintegration

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

// This gate builds the replacement as the actual pinned cmd/compile, then asks
// cmd/go to rebuild sources with it. No tool-exec fallback or prebuilt runtime
// archives stand in for calls into the library.
func TestPinnedCompilerDriver(t *testing.T) {
	repo := os.Getenv("GO_SSA_SOURCE_REPO")
	if repo == "" || os.Getenv("GO_SSA_DRIVER_INTEGRATION") == "" {
		t.Skip("set GO_SSA_SOURCE_REPO and GO_SSA_DRIVER_INTEGRATION")
	}
	image := os.Getenv("GO_SSA_LINUX_AMD64_IMAGE")
	if (runtime.GOOS != "linux" || runtime.GOARCH != "amd64") && image == "" {
		t.Fatal("set GO_SSA_LINUX_AMD64_IMAGE for target execution")
	}
	root, e := filepath.Abs("../..")
	if e != nil {
		t.Fatal(e)
	}
	dir := t.TempDir()
	tree := filepath.Join(dir, "go")
	bootstrap := runtime.GOROOT()
	manifest, e := Prepare(context.Background(), repo, root, tree, bootstrap)
	if e != nil {
		t.Fatal(e)
	}
	hostTools := filepath.Join(tree, "pkg/tool", runtime.GOOS+"_"+runtime.GOARCH)
	env := func(goroot, cache, goos, goarch string) []string {
		return append(os.Environ(), "GOROOT="+goroot, "GOCACHE="+cache, "GOTOOLCHAIN=local", "GOWORK=off", "GOFLAGS=", "GOOS="+goos, "GOARCH="+goarch, "GOAMD64=v1", "GOEXPERIMENT=", "CGO_ENABLED=0", "GOPROXY=off", "GOSUMDB=off", "GOMAXPROCS=4", "GOCOMPILEDEBUG=", "GOSSAFUNC=", "GOSSADIR=", "GO_SSA_BRIDGE_TRACE=")
	}
	run := func(executable string, args []string, environment []string, cwd string) []byte {
		t.Helper()
		cmd := exec.Command(executable, args...)
		cmd.Env = environment
		cmd.Dir = cwd
		b, e := cmd.CombinedOutput()
		if e != nil {
			t.Fatalf("%s %v: %v\n%s", executable, args, e, b)
		}
		return b
	}
	built := filepath.Join(hostTools, "compile.library")
	run(filepath.Join(tree, "bin/go"), []string{"build", "-p=4", "-pgo=off", "-o", built, "cmd/compile"}, env(tree, filepath.Join(dir, "host-cache"), runtime.GOOS, runtime.GOARCH), filepath.Join(tree, "src/cmd"))
	if e = os.Rename(filepath.Join(hostTools, "compile"), filepath.Join(hostTools, "compile.upstream")); e != nil {
		t.Fatal(e)
	}
	if e = os.Rename(built, filepath.Join(hostTools, "compile")); e != nil {
		t.Fatal(e)
	}
	compiler := filepath.Join(hostTools, "compile")
	targetEnv := env(tree, filepath.Join(dir, "library-cache"), "linux", "amd64")
	identity := run(compiler, []string{"-V=full"}, targetEnv, dir)
	if !bytes.Contains(identity, []byte(manifest.Library)) {
		t.Fatalf("compiler identity lacks library hash: %s", identity)
	}
	probe := filepath.Join(dir, "probe")
	if e = os.Mkdir(probe, 0755); e != nil {
		t.Fatal(e)
	}
	for name, data := range map[string]string{
		"go.mod": "module example/bridgeprobe\n\ngo 1.27\n",
		"main.go": `package main
import("fmt";"reflect";"slices")
func twice[T ~int](v T)T{return v*2}
func recovered()(n int){defer func(){if recover()!=nil{n=11}}();panic("check")}
func main(){x:=[]int{5,1,3};slices.Sort(x);ch:=make(chan int);go func(){ch<-twice(x[2])}();fmt.Println(x,<-ch,reflect.DeepEqual(x,[]int{1,3,5}),recovered())}
`,
	} {
		if e = os.WriteFile(filepath.Join(probe, name), []byte(data), 0600); e != nil {
			t.Fatal(e)
		}
	}
	trace := filepath.Join(dir, "calls.jsonl")
	targetEnv = append(targetEnv, "GO_SSA_BRIDGE_TRACE="+trace)
	run(filepath.Join(tree, "bin/go"), []string{"build", "-a", "-p=4", "-o", "library", "."}, targetEnv, probe)
	records, e := os.ReadFile(trace)
	if e != nil {
		t.Fatal(e)
	}
	packages := map[string]bool{}
	for _, line := range bytes.Split(bytes.TrimSpace(records), []byte{'\n'}) {
		var record struct{ Package, Library, Upstream string }
		if e = json.Unmarshal(line, &record); e != nil {
			t.Fatal(e)
		}
		if record.Library != manifest.Library || record.Upstream != manifest.Upstream {
			t.Fatal("unexpected compiler provenance", record)
		}
		packages[record.Package] = true
	}
	for _, path := range []string{"runtime", "fmt", "reflect", "main"} {
		if !packages[path] {
			t.Fatalf("no library compile evidence for %s", path)
		}
	}
	execute := func(binary string, args ...string) []byte {
		t.Helper()
		if runtime.GOOS == "linux" && runtime.GOARCH == "amd64" {
			return run(filepath.Join(probe, binary), args, targetEnv, probe)
		}
		dockerArgs := []string{"run", "--rm", "--pull=never", "--platform", "linux/amd64", "--network", "none", "--mount", "type=bind,source=" + probe + ",target=/work", "-e", "GOOS=linux", "-e", "GOARCH=amd64", "-e", "GOAMD64=v1", image}
		dockerArgs = append(dockerArgs, "/work/"+binary)
		dockerArgs = append(dockerArgs, args...)
		return run("docker", dockerArgs, os.Environ(), probe)
	}
	ours := execute("library")
	if string(ours) != "[1 3 5] 10 true 11\n" {
		t.Fatalf("wrong program output %q", ours)
	}
	run(filepath.Join(bootstrap, "bin/go"), []string{"build", "-p=4", "-o", "official", "."}, env(bootstrap, filepath.Join(dir, "official-cache"), "linux", "amd64"), probe)
	if native := execute("official"); !bytes.Equal(native, ours) {
		t.Fatalf("execution differs: library=%q native=%q", ours, native)
	}
	// Unsupported options cannot silently invoke the original compiler or
	// produce an archive using ignored semantics.
	rejected := filepath.Join(probe, "rejected.a")
	cmd := exec.Command(compiler, "-race", "-p", "main", "-o", rejected, filepath.Join(probe, "main.go"))
	cmd.Env = targetEnv
	if b, e := cmd.CombinedOutput(); e == nil || !bytes.Contains(b, []byte("unsupported go-backend-ssa compiler option -race")) {
		t.Fatalf("unsupported option was not rejected: %v %s", e, b)
	}
	if _, e = os.Stat(rejected); !os.IsNotExist(e) {
		t.Fatal("rejected invocation produced output", e)
	}
	t.Logf("%d packages including runtime compiled through library; program matches official Go", len(packages))
	if os.Getenv("GO_SSA_DRIVER_FULL") == "" {
		return
	}
	stdTrace := filepath.Join(dir, "std.jsonl")
	fullEnv := append(targetEnv, "GO_SSA_BRIDGE_TRACE="+stdTrace)
	run(filepath.Join(tree, "bin/go"), []string{"build", "-a", "-p=4", "std"}, fullEnv, probe)
	b, e := os.ReadFile(stdTrace)
	if e != nil {
		t.Fatal(e)
	}
	builtPackages := map[string]bool{}
	for _, line := range bytes.Split(bytes.TrimSpace(b), []byte{'\n'}) {
		var record struct{ Package, Library, Upstream string }
		if e = json.Unmarshal(line, &record); e != nil {
			t.Fatal(e)
		}
		if record.Library != manifest.Library || record.Upstream != manifest.Upstream {
			t.Fatal("unexpected standard library provenance", record)
		}
		builtPackages[record.Package] = true
	}
	listed := run(filepath.Join(tree, "bin/go"), []string{"list", "-deps", "-json", "std"}, targetEnv, probe)
	decoder := json.NewDecoder(bytes.NewReader(listed))
	expected := 0
	for decoder.More() {
		var pkg struct {
			ImportPath string
			GoFiles    []string
		}
		if e = decoder.Decode(&pkg); e != nil {
			t.Fatal(e)
		}
		if pkg.ImportPath == "unsafe" || len(pkg.GoFiles) == 0 {
			continue
		}
		expected++
		if !builtPackages[pkg.ImportPath] {
			t.Fatalf("standard package did not compile through library: %s", pkg.ImportPath)
		}
	}
	if expected != len(builtPackages) {
		t.Fatalf("standard rebuild package count: got %d want %d", len(builtPackages), expected)
	}
	t.Logf("all %d selected standard packages compiled through library", expected)
	run(filepath.Join(tree, "bin/go"), []string{"build", "-a", "-p=4", "-o", filepath.Join(probe, "compile.self"), "cmd/compile"}, targetEnv, filepath.Join(tree, "src/cmd"))
	pgoCalls, e := os.ReadFile(trace)
	if e != nil {
		t.Fatal(e)
	}
	pgoUsed := false
	for _, line := range bytes.Split(bytes.TrimSpace(pgoCalls), []byte{'\n'}) {
		var call struct{ Package, Profile string }
		if e = json.Unmarshal(line, &call); e != nil {
			t.Fatal(e)
		}
		pgoUsed = pgoUsed || call.Package == "main" && call.Profile != ""
	}
	if !pgoUsed {
		t.Fatal("compiler self-build did not use the native default PGO profile")
	}
	t.Log("compiler self-build used the native default PGO profile")
	if b := execute("compile.self", "-V=full"); !bytes.Contains(b, []byte(manifest.Library)) {
		t.Fatalf("self-compiled compiler identity mismatch: %s", b)
	}
	selfSource := filepath.Join(probe, "self.go")
	if e = os.WriteFile(selfSource, []byte("package p;func F(x int)int{return x*7+3}"), 0600); e != nil {
		t.Fatal(e)
	}
	hostArchive := filepath.Join(probe, "host.a")
	run(compiler, []string{"-p", "p", "-trimpath", probe + "=>/source", "-o", hostArchive, selfSource}, targetEnv, probe)
	if runtime.GOOS == "linux" && runtime.GOARCH == "amd64" {
		execute("compile.self", "-p", "p", "-trimpath", probe+"=>/source", "-o", filepath.Join(probe, "self.a"), selfSource)
	} else {
		execute("compile.self", "-p", "p", "-trimpath", "/work=>/source", "-o", "/work/self.a", "/work/self.go")
	}
	host, e := os.ReadFile(hostArchive)
	if e != nil {
		t.Fatal(e)
	}
	self, e := os.ReadFile(filepath.Join(probe, "self.a"))
	if e != nil {
		t.Fatal(e)
	}
	if !bytes.Equal(host, self) {
		t.Fatalf("self-compiled compiler output differs: host=%d self=%d", len(host), len(self))
	}
	t.Logf("self-compiled linux/amd64 compiler emitted the same %d-byte archive", len(self))
}

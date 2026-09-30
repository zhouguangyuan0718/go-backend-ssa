package compiler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
)

// This oracle changes only the official exporter's object-index policy. The
// official source frontend, optimization, encoding and machine backend remain
// independent of the library and its migration transform.
func TestArtifactOfficialExportAndExecution(t *testing.T) {
	if os.Getenv("GO_SSA_NATIVE_EXECUTION") == "" {
		t.Skip("set GO_SSA_NATIVE_EXECUTION for official named export and execution")
	}
	dir := t.TempDir()
	env := []string{"GOTOOLCHAIN=go1.27.1", "GOWORK=off", "GO111MODULE=off", "GOFLAGS=", "GOOS=linux", "GOARCH=amd64", "GOAMD64=v1", "GOEXPERIMENT=", "CGO_ENABLED=0", "GOPROXY=off", "GOSUMDB=off", "GOCOMPILEDEBUG=", "GOSSAFUNC="}
	run := func(executable string, extra []string, args ...string) []byte {
		t.Helper()
		cmd := exec.Command(executable, args...)
		cmd.Dir = dir
		cmd.Env = append(append(os.Environ(), env...), extra...)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("%s %v: %v\n%s", executable, args, err, out)
		}
		return out
	}
	write := func(name string, b []byte) {
		t.Helper()
		if err := os.WriteFile(name, b, 0600); err != nil {
			t.Fatal(err)
		}
	}
	root := strings.TrimSpace(string(run("go", nil, "env", "GOROOT")))
	original := filepath.Join(root, "src/cmd/compile/internal/noder/linker.go")
	source, err := os.ReadFile(original)
	if err != nil {
		t.Fatal(err)
	}
	anchor := []byte("func (l *linker) lsymIdx(w *pkgbits.Encoder, linkname string, lsym *obj.LSym) bool {")
	if bytes.Count(source, anchor) != 1 {
		t.Fatal("official export policy anchor changed")
	}
	overlaySource := filepath.Join(dir, "linker.go")
	write(overlaySource, bytes.Replace(source, anchor, append(bytes.Clone(anchor), []byte("\nlinkname = \"independent-export\"\n")...), 1))
	overlayFile := filepath.Join(dir, "overlay.json")
	overlay, _ := json.Marshal(map[string]any{"Replace": map[string]string{original: overlaySource}})
	write(overlayFile, overlay)
	tool := filepath.Join(dir, "named-compile")
	run(filepath.Join(root, "bin/go"), []string{"GOROOT=" + root, "GOTOOLCHAIN=local", "GOMODCACHE=" + filepath.Join(dir, "modcache"), "GOOS=" + runtime.GOOS, "GOARCH=" + runtime.GOARCH}, "build", "-p=4", "-overlay", overlayFile, "-o", tool, "cmd/compile")
	const leaf = `package leaf
 type Value struct{X int; Label string}
 func(v Value)Get()int{return v.X}
 var Count=3
 func init(){Count++}
 func hidden(x int)int{return x+Count}
 func Add[T ~int](x,y T)T{return x+y}
 func Twice(x int)int{return hidden(x)+hidden(x)}
 func Closure(x int)func()int{return func()int{return x+Count}}
 func Touch(p *int){defer func(){*p+=2}()}
 `
	file := filepath.Join(dir, "leaf.go")
	write(file, []byte(leaf))
	request := func(path, file, source string) Request {
		return Request{ImportPath: path, Files: []SourceFile{{Name: file, Data: []byte(source)}}, Target: Target{GOOS: "linux", GOARCH: "amd64", GOAMD64: 1}, Options: Options{Complete: true, DisableDwarf: true}}
	}
	exportPayload := func(archive string) []byte {
		t.Helper()
		member := run("go", nil, "tool", "pack", "p", archive, "__.PKGDEF")
		marker := []byte("\n$$B\nu")
		i := bytes.Index(member, marker)
		if i < 0 || !bytes.HasSuffix(member, []byte("\n$$\n")) {
			t.Fatal("official package definition framing")
		}
		return member[i+len(marker) : len(member)-4]
	}
	var artifact *Artifact
	var object string
	for _, mode := range []string{"default", "noinline", "noopt"} {
		t.Run(mode, func(t *testing.T) {
			req := request("leaf", file, leaf)
			args := []string{"-p", "leaf", "-complete", "-dwarf=false"}
			switch mode {
			case "noinline":
				req.Options.DisableInlining = true
				args = append(args, "-l")
			case "noopt":
				req.Options.DisableOptimization = true
				args = append(args, "-N")
			}
			p, err := Build(context.Background(), req)
			if err != nil {
				t.Fatal(err)
			}
			archive := filepath.Join(dir, mode+"-leaf.a")
			run(tool, nil, append(args, "-o", archive, file)...)
			want := exportPayload(archive)
			if !bytes.Equal([]byte(p.Artifact.payload), want) {
				i := 0
				for i < len(want) && i < len(p.Artifact.payload) && want[i] == p.Artifact.payload[i] {
					i++
				}
				t.Fatalf("named export differs at %d: library=%d official=%d", i, len(p.Artifact.payload), len(want))
			}
			t.Logf("all %d Unified IR bytes match official named export", len(want))
			if mode == "default" {
				artifact = p.Artifact
				object = archive
			}
		})
	}
	if artifact == nil {
		t.Fatal("default export failed")
	}
	// Explicit native-archive input: imported extension records retain their
	// original indices, while local definitions use the named export policy.
	nativeLeaf := filepath.Join(dir, "native-leaf.a")
	run("go", nil, "tool", "compile", "-p", "leaf", "-complete", "-dwarf=false", "-o", nativeLeaf, file)
	nativeBytes, err := os.ReadFile(nativeLeaf)
	if err != nil {
		t.Fatal(err)
	}
	const mid = `package mid;import "leaf";type Alias=leaf.Value;func F(v Alias)int{return v.Get()+leaf.Twice(3)};func Add[T ~int](a,b T)T{return leaf.Add(a,b)}`
	midFile := filepath.Join(dir, "mid.go")
	write(midFile, []byte(mid))
	midCfg := filepath.Join(dir, "mid.cfg")
	write(midCfg, []byte("packagefile leaf="+nativeLeaf+"\n"))
	midReq := request("mid", midFile, mid)
	midReq.Imports = map[string][]byte{"leaf": nativeBytes}
	p, err := Build(context.Background(), midReq)
	if err != nil {
		t.Fatal(err)
	}
	midArchive := filepath.Join(dir, "mid.a")
	run(tool, nil, "-p", "mid", "-complete", "-dwarf=false", "-importcfg", midCfg, "-o", midArchive, midFile)
	if !bytes.Equal([]byte(p.Artifact.payload), exportPayload(midArchive)) || !p.Artifact.Info().NativeObjectReferences {
		t.Fatal("native archive adapter export differs")
	}

	const main = `package main
 import "leaf"
 func main(){var v interface{Get()int}=leaf.Value{X:9};x:=0;leaf.Touch(&x);println(leaf.Add(3,4),leaf.Twice(5),v.Get(),leaf.Closure(6)(),x)}
 `
	mainFile := filepath.Join(dir, "main.go")
	write(mainFile, []byte(main))
	req := request("main", mainFile, main)
	req.Artifacts = map[string]*Artifact{"leaf": artifact}
	result, err := Compile(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	oursArchive := filepath.Join(dir, "main-library.a")
	write(oursArchive, result.Archive)
	mainCfg := filepath.Join(dir, "main.cfg")
	write(mainCfg, []byte("packagefile leaf="+object+"\n"))
	referenceArchive := filepath.Join(dir, "main-reference.a")
	run("go", nil, "tool", "compile", "-p", "main", "-complete", "-dwarf=false", "-importcfg", mainCfg, "-o", referenceArchive, mainFile)
	expected, err := os.ReadFile(referenceArchive)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(result.Archive, expected) {
		t.Fatalf("artifact consumer archive differs: library=%d official=%d", len(result.Archive), len(expected))
	}
	packages := map[string]string{"leaf": object}
	decoder := json.NewDecoder(bytes.NewReader(run("go", nil, "list", "-deps", "-export", "-json", "runtime")))
	for {
		var pkg struct{ ImportPath, Export string }
		err := decoder.Decode(&pkg)
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if pkg.Export != "" {
			packages[pkg.ImportPath] = pkg.Export
		}
	}
	names := make([]string, 0, len(packages))
	for name := range packages {
		names = append(names, name)
	}
	slices.Sort(names)
	var config strings.Builder
	for _, name := range names {
		fmt.Fprintf(&config, "packagefile %s=%s\n", name, packages[name])
	}
	linkCfg := filepath.Join(dir, "link.cfg")
	write(linkCfg, []byte(config.String()))
	binary := filepath.Join(dir, "artifact-program")
	run("go", nil, "tool", "link", "-importcfg", linkCfg, "-o", binary, oursArchive)
	var output []byte
	if runtime.GOOS == "linux" && runtime.GOARCH == "amd64" {
		output = run(binary, nil)
	} else {
		image := os.Getenv("GO_SSA_LINUX_AMD64_IMAGE")
		if image == "" {
			t.Fatal("set GO_SSA_LINUX_AMD64_IMAGE")
		}
		output = run("docker", nil, "run", "--rm", "--pull=never", "--platform", "linux/amd64", "--network", "none", "--mount", "type=bind,source="+dir+",target=/work,readonly", image, "/work/artifact-program")
	}
	if string(output) != "7 18 9 10 2\n" {
		t.Fatalf("artifact program output %q", output)
	}
	t.Logf("artifact consumer archive matches official compiler; linked program returned %s", strings.TrimSpace(string(output)))
}

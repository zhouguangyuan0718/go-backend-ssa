package compiler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"go-backend-ssa/compiler"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
)

func TestNativeCompileLinkAndRun(t *testing.T) {
	if os.Getenv("GO_SSA_NATIVE_EXECUTION") == "" {
		t.Skip("set GO_SSA_NATIVE_EXECUTION for official-toolchain link and execution")
	}
	image := os.Getenv("GO_SSA_LINUX_AMD64_IMAGE")
	if (runtime.GOOS != "linux" || runtime.GOARCH != "amd64") && image == "" {
		t.Fatal("set GO_SSA_LINUX_AMD64_IMAGE to an installed Linux AMD64 image")
	}
	dir := t.TempDir()
	run := func(t *testing.T, args ...string) []byte {
		t.Helper()
		cmd := exec.Command("go", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GOTOOLCHAIN=go1.27.1", "GOWORK=off", "GOOS=linux", "GOARCH=amd64", "GOAMD64=v1", "GOEXPERIMENT=", "CGO_ENABLED=0", "GO111MODULE=off", "GOFLAGS=", "GOPROXY=off", "GOSUMDB=off")
		b, e := cmd.CombinedOutput()
		if e != nil {
			t.Fatalf("go %v: %v\n%s", args, e, b)
		}
		return b
	}
	data := run(t, "list", "-deps", "-export", "-json", "runtime")
	decoder := json.NewDecoder(bytes.NewReader(data))
	packages := map[string]string{}
	for {
		var pkg struct{ ImportPath, Export string }
		e := decoder.Decode(&pkg)
		if e == io.EOF {
			break
		}
		if e != nil {
			t.Fatal(e)
		}
		if pkg.Export != "" {
			packages[pkg.ImportPath] = pkg.Export
		}
	}
	if packages["runtime"] == "" {
		t.Fatal("runtime archive missing")
	}
	depSource := `package dep
import "unsafe"
func FromUintptr(u uintptr)*int{return (*int)(unsafe.Pointer(u))}
var Offset=3
func Twice[T ~int](x T)T{return x*2}
type Value int
type Header struct{ _ byte; A int64; B [3]byte }
const Word=17
const Fraction=1.5
func(x Value)Get()int{return int(x)+Offset}
`
	mainSource := `package main
import "example/dep"
var M=map[int]int{1:7,2:9}
func caught()(n int){defer func(){if recover()!=nil{n=11}}();panic("check")}
func main(){ch:=make(chan int);go func(){ch<-dep.Twice(M[1])}();var v interface{Get()int}=dep.Value(5);sum:=0;var fs []func()int;for i:=range 4{fs=append(fs,func()int{return i})};for _,f:=range fs{sum+=f()};println(<-ch,v.Get(),sum,caught())}
`
	depFile, mainFile := filepath.Join(dir, "dep.go"), filepath.Join(dir, "main.go")
	for file, source := range map[string]string{depFile: depSource, mainFile: mainSource} {
		if e := os.WriteFile(file, []byte(source), 0600); e != nil {
			t.Fatal(e)
		}
	}
	cfg := func(t *testing.T, name string, extra map[string]string) string {
		t.Helper()
		all := map[string]string{}
		for k, v := range packages {
			all[k] = v
		}
		for k, v := range extra {
			all[k] = v
		}
		keys := make([]string, 0, len(all))
		for k := range all {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		var b strings.Builder
		for _, k := range keys {
			fmt.Fprintf(&b, "packagefile %s=%s\n", k, all[k])
		}
		file := filepath.Join(dir, name)
		if e := os.WriteFile(file, []byte(b.String()), 0600); e != nil {
			t.Fatal(e)
		}
		return file
	}
	execute := func(t *testing.T, binary string) []byte {
		t.Helper()
		var cmd *exec.Cmd
		if runtime.GOOS == "linux" && runtime.GOARCH == "amd64" {
			cmd = exec.Command(binary)
		} else {
			cmd = exec.Command("docker", "run", "--rm", "--platform", "linux/amd64", "--network", "none", "--mount", "type=bind,source="+dir+",target=/work,readonly", image, "/work/"+filepath.Base(binary))
		}
		b, e := cmd.CombinedOutput()
		if e != nil {
			t.Fatalf("execute %s: %v\n%s", binary, e, b)
		}
		return b
	}
	for _, mode := range []string{"default", "noopt", "nodwarf", "driver"} {
		t.Run(mode, func(t *testing.T) {
			opts := compiler.Options{Complete: true, Workers: 4}
			depHeader, mainHeader := filepath.Join(dir, mode+"-dep.h"), filepath.Join(dir, mode+"-main.h")
			args := []string{"tool", "compile", "-p", "example/dep", "-complete"}
			switch mode {
			case "noopt":
				opts.DisableInlining = true
				opts.DisableOptimization = true
				args = append(args, "-N", "-l")
			case "driver":
				opts.AssemblyHeader = true
				opts.BuildID = "explicit-driver-build-id"
				opts.TrimPath = dir + "=>module"
				opts.Language = "go1.22"
				args = append(args, "-asmhdr", depHeader, "-buildid", opts.BuildID, "-trimpath", opts.TrimPath, "-lang", opts.Language)
			case "nodwarf":
				opts.DisableDwarf = true
				args = append(args, "-dwarf=false")
			}
			refDep := filepath.Join(dir, mode+"-reference-dep.a")
			run(t, append(args, "-o", refDep, depFile)...)
			req := request("example/dep", depSource)
			req.Files[0].Name = depFile
			req.Options = opts
			dep, e := compiler.Compile(context.Background(), req)
			if e != nil {
				t.Fatal(e)
			}
			if opts.AssemblyHeader {
				header, e := os.ReadFile(depHeader)
				if e != nil {
					t.Fatal(e)
				}
				if !bytes.Equal(dep.AssemblyHeader, header) {
					t.Fatalf("assembly header differs: library=%s native=%s", dep.AssemblyHeader, header)
				}
			}
			refBytes, e := os.ReadFile(refDep)
			if e != nil {
				t.Fatal(e)
			}
			if !bytes.Equal(dep.Archive, refBytes) {
				t.Errorf("dependency archive differs: library=%d native=%d", len(dep.Archive), len(refBytes))
				_ = os.WriteFile(filepath.Join(dir, mode+"-library-dep.a"), dep.Archive, 0600)
			}
			req = request("main", mainSource)
			req.Files[0].Name = mainFile
			req.Options = opts
			req.Imports = map[string][]byte{"example/dep": dep.Archive}
			result, e := compiler.Compile(context.Background(), req)
			if e != nil {
				t.Fatal(e)
			}
			archive := filepath.Join(dir, mode+"-library-main.a")
			if e = os.WriteFile(archive, result.Archive, 0600); e != nil {
				t.Fatal(e)
			}
			libraryDep := filepath.Join(dir, mode+"-library-dep.a")
			if e = os.WriteFile(libraryDep, dep.Archive, 0600); e != nil {
				t.Fatal(e)
			}
			libraryCfg := cfg(t, mode+"-library.cfg", map[string]string{"example/dep": libraryDep})
			binary := filepath.Join(dir, mode+"-library")
			run(t, "tool", "link", "-importcfg", libraryCfg, "-o", binary, archive)
			ours := execute(t, binary)
			if string(ours) != "14 8 6 11\n" {
				t.Fatalf("unexpected native program output %q", ours)
			}
			referenceCfg := cfg(t, mode+"-reference.cfg", map[string]string{"example/dep": refDep})
			refMain := filepath.Join(dir, mode+"-reference-main.a")
			args = []string{"tool", "compile", "-p", "main", "-complete", "-importcfg", referenceCfg, "-o", refMain}
			switch mode {
			case "noopt":
				args = append(args, "-N", "-l")
			case "driver":
				args = append(args, "-asmhdr", mainHeader, "-buildid", opts.BuildID, "-trimpath", opts.TrimPath, "-lang", opts.Language)
			case "nodwarf":
				args = append(args, "-dwarf=false")
			}
			run(t, append(args, mainFile)...)
			if opts.AssemblyHeader {
				header, e := os.ReadFile(mainHeader)
				if e != nil {
					t.Fatal(e)
				}
				if !bytes.Equal(result.AssemblyHeader, header) {
					t.Fatalf("main assembly header differs: library=%s native=%s", result.AssemblyHeader, header)
				}
			}
			refMainBytes, e := os.ReadFile(refMain)
			if e != nil {
				t.Fatal(e)
			}
			if !bytes.Equal(result.Archive, refMainBytes) {
				t.Errorf("main archive differs: library=%d native=%d", len(result.Archive), len(refMainBytes))
			}
			refBinary := filepath.Join(dir, mode+"-reference")
			run(t, "tool", "link", "-importcfg", referenceCfg, "-o", refBinary, refMain)
			if native := execute(t, refBinary); !bytes.Equal(ours, native) {
				t.Fatalf("execution differs: library=%q native=%q", ours, native)
			}
			t.Logf("library frontend/backend archive linked and executed: %s", strings.TrimSpace(string(ours)))
		})
	}
}

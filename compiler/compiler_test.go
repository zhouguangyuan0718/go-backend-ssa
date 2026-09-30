package compiler_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"go-backend-ssa/compiler"
	"io/fs"
	"strings"
	"testing"
	"testing/fstest"
)

func request(path, source string) compiler.Request {
	return compiler.Request{ImportPath: path, Target: compiler.Target{GOOS: "linux", GOARCH: "amd64", GOAMD64: 1}, Files: []compiler.SourceFile{{Name: "input.go", Data: []byte(source)}}}
}
func TestCompileSourceAndDependency(t *testing.T) {
	for i := range 8 {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			t.Parallel()
			depReq := request("example/dep", "package dep;func Add[T ~int](a,b T)T{return a+b};type Pair[T any]struct{A,B T};func(p Pair[T])First()T{return p.A}")
			depReq.Options.DisableDwarf = i%3 == 1
			depReq.Options.DisableOptimization = i%3 == 2
			depReq.Options.DisableInlining = i%3 == 2
			dep, e := compiler.Compile(context.Background(), depReq)
			if e != nil {
				t.Fatal(e)
			}
			req := request("main", fmt.Sprintf(`package main
import "example/dep"
var M=map[int]int{1:%d}
func main(){p:=dep.Pair[int]{A:dep.Add(M[1],2)};println(p.First())}
`, i+1))
			req.Imports = map[string][]byte{"example/dep": dep.Archive}
			req.Options = depReq.Options
			req.Options.Workers = 4
			result, e := compiler.Compile(context.Background(), req)
			if e != nil {
				t.Fatal(e)
			}
			if result.PackageName != "main" || !bytes.HasPrefix(result.Archive, []byte("!<arch>\n")) {
				t.Fatal("invalid package archive")
			}
			saved := bytes.Clone(result.Archive)
			clear(dep.Archive)
			req.Files[0].Data = []byte("package main;func main(){println(99)}")
			delete(req.Imports, "example/dep")
			next, e := compiler.Compile(context.Background(), req)
			if e != nil {
				t.Fatal(e)
			}
			if !bytes.Equal(saved, result.Archive) || bytes.Equal(next.Archive, result.Archive) {
				t.Fatal("compiled results share state")
			}
		})
	}
}
func TestCompileErrorsAndCancellation(t *testing.T) {
	for _, source := range []string{"package p;func Bad(){return 3}", "package p;func Bad(", "package p;import \"missing\";var X=missing.X"} {
		result, e := compiler.Compile(context.Background(), request("example/p", source))
		if e == nil || result != nil {
			t.Errorf("invalid source accepted: %q %v", source, e)
		}
	}
	if result, e := compiler.Compile(context.Background(), request("example/p", "package p;func TooLarge(x [1<<30]byte){}")); result != nil || e == nil || !strings.Contains(e.Error(), "stack frame too large") {
		t.Fatal("final native stack check did not withhold output", result, e)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if result, e := compiler.Compile(ctx, request("p", "package p")); result != nil || !errors.Is(e, context.Canceled) {
		t.Fatal(result, e)
	}
	for _, target := range []compiler.Target{{}, {GOOS: "linux", GOARCH: "arm64", GOAMD64: 1}, {GOOS: "linux", GOARCH: "amd64", GOAMD64: 1, Experiments: "unknown"}} {
		req := request("p", "package p")
		req.Target = target
		if result, e := compiler.Compile(context.Background(), req); e == nil || result != nil {
			t.Fatal("invalid target accepted", target)
		}
	}
	if result, e := compiler.Compile(context.Background(), request("p", "package p;func Good()int{return 1}")); e != nil || result == nil {
		t.Fatal("failure poisoned subsequent compilation", e)
	}
}
func TestCompileDiagnosticPosition(t *testing.T) {
	_, e := compiler.Compile(context.Background(), request("p", "package p\nfunc F(){\nreturn 1\n}"))
	if e == nil || !strings.Contains(e.Error(), "input.go:3:") {
		t.Fatal("missing source position", e)
	}
}

func TestCompileEmbeddedDataAndProviderFailure(t *testing.T) {
	dependency, e := compiler.Compile(context.Background(), request("embed", "package embed"))
	if e != nil {
		t.Fatal(e)
	}
	req := request("p", "package p\nimport _ \"embed\"\n//go:embed data\nvar Data string\nfunc Len()int{return len(Data)}")
	req.Imports = map[string][]byte{"embed": dependency.Archive}
	req.Embed.Patterns = map[string][]string{"data": {"data"}}
	req.Embed.Files = map[string]string{"data": "data"}
	data := strings.Repeat("owned embedded contents", 200)
	files := fstest.MapFS{"data": &fstest.MapFile{Data: []byte(data)}}
	req.Embed.Open = files.Open
	result, e := compiler.Compile(context.Background(), req)
	if e != nil {
		t.Fatal(e)
	}
	if !bytes.Contains(result.Archive, []byte(data)) {
		t.Fatal("large embedded data missing from archive")
	}
	marker := new(int)
	req.Embed.Open = func(string) (fs.File, error) { panic(marker) }
	func() {
		defer func() {
			if p := recover(); p != marker {
				t.Errorf("provider panic was changed or swallowed: %v", p)
			}
		}()
		_, _ = compiler.Compile(context.Background(), req)
		t.Error("provider panic accepted")
	}()
	ctx, cancel := context.WithCancel(context.Background())
	req.Embed.Open = func(name string) (fs.File, error) { cancel(); return files.Open(name) }
	if result, e := compiler.Compile(ctx, req); result != nil || !errors.Is(e, context.Canceled) {
		t.Fatal("cancelled compilation returned output", result, e)
	}
	req.Embed.Open = func(string) (fs.File, error) { return nil, fs.ErrNotExist }
	if result, e := compiler.Compile(context.Background(), req); e == nil || result != nil {
		t.Fatal("missing embedded input accepted")
	}
}

func TestNativeDriverOptions(t *testing.T) {
	req := request("internal/runtime/atomic", "package atomic;func F(x int)int{return x*4+7}")
	req.Options.Standard = true
	optimized, err := compiler.Compile(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	req.Options.DisableOptimization = true
	forced, err := compiler.Compile(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(optimized.Archive, forced.Archive) {
		t.Fatal("native runtime optimization normalization differs")
	}
	req = request("p", "package p;func F(){for range 4{}}")
	req.Options.Language = "go1.21"
	if result, err := compiler.Compile(context.Background(), req); result != nil || err == nil || !strings.Contains(err.Error(), "go1.22") {
		t.Fatal("language version was ignored", result, err)
	}
	req.Options.Language = "go1.22"
	if _, err := compiler.Compile(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	for _, language := range []string{"nonsense", "go1.28", "go1.22.1"} {
		req.Options.Language = language
		if result, err := compiler.Compile(context.Background(), req); result != nil || err == nil {
			t.Fatal("invalid language version accepted", language)
		}
	}
	req = request("p", "package p; const C=17; const F=1.5; type Header struct{_ byte;A int64;B [3]byte}")
	req.Options.AssemblyHeader = true
	result, err := compiler.Compile(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	header := string(result.AssemblyHeader)
	for _, want := range []string{"#define const_C 17", "#define Header__size 24", "#define Header_A 8", "#define Header_B 16"} {
		if !strings.Contains(header, want) {
			t.Fatalf("missing %q in %s", want, header)
		}
	}
	if strings.Contains(header, "const_F") || strings.Contains(header, "Header__ ") {
		t.Fatal("unsupported/blank header declaration", header)
	}
}

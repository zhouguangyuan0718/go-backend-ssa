package compiler_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"go-backend-ssa/compiler"
)

func TestNativeErrorReportOracle(t *testing.T) {
	many := "package p\n"
	for i := range 15 {
		many += fmt.Sprintf("var V%d = missing%d\n", i, i)
	}
	for _, example := range []struct{ name, source string }{
		{"syntax", "package p\nfunc F( {\nvar x =\n"},
		{"types", "package p\nvar A int = true\nvar B = missing\nfunc F(){var x string = 3; _=x}\n"},
		{"limit", many},
		{"channel-layout", "package p\nvar _ chan [0x2FFFF]byte\nvar _ = make(chan [0x2FFFF]byte)\nvar A chan [0x2FFFF]byte\nvar B = make(chan [0x2FFFF]byte)\n"},
		{"runtime-closures", "package p\nfunc f(x int)func(int)int{return func(y int)int{return x+y}}\nfunc g(x int)func(int)int{return func(y int)int{x+=y;return x}}\n"},
		{"line", "//line :10\n"},
		{"line-types", "package p\n//line virtual.go:20:7\nvar A int = true\nvar B = missing\n"},
		{"duplicate", "package p\nvar A = []int{missing,missing,missing}\n"},
		{"directive", "package p\n//go:noinline\nvar A int = true\n"},
	} {
		file := filepath.Join(t.TempDir(), example.name+".go")
		if err := os.WriteFile(file, []byte(example.source), 0600); err != nil {
			t.Fatal(err)
		}
		for _, arch := range []string{"amd64", "arm64"} {
			for _, mode := range []struct {
				name               string
				all, noCol, actual bool
			}{
				{"default", false, false, false}, {"all-no-columns", true, true, false}, {"actual-files", true, false, true},
			} {
				t.Run(example.name+"/"+arch+"/"+mode.name, func(t *testing.T) {
					t.Parallel()
					req := request("p", example.source)
					req.Files[0].Name = file
					req.Target = compiler.Target{GOOS: "linux", GOARCH: arch, GOAMD64: 1}
					if arch == "arm64" {
						req.Target.GOAMD64 = 0
						req.Target.GOARM64 = "v8.0"
					}
					req.Options.CompilingRuntime = example.name == "runtime-closures"
					req.Options.UnlimitedErrors = mode.all
					req.Options.OmitErrorColumns = mode.noCol
					req.Options.ActualErrorFiles = mode.actual
					result, err := compiler.Compile(context.Background(), req)
					var failure *compiler.Error
					if result != nil || !errors.As(err, &failure) || len(failure.Diagnostics) == 0 || len(failure.Report) == 0 {
						t.Fatalf("missing source diagnostics: %v", err)
					}
					saved := bytes.Clone(failure.Report)
					for _, stage := range []compiler.Stage{compiler.Built, compiler.EarlyOptimized} {
						builtReq := req
						builtReq.Options.Stage = stage
						pkg, buildErr := compiler.Build(context.Background(), builtReq)
						var buildFailure *compiler.Error
						if pkg != nil || !errors.As(buildErr, &buildFailure) || !bytes.Equal(saved, buildFailure.Report) {
							t.Fatalf("%s source errors differ: %v", stage, buildErr)
						}
					}
					if example.name == "limit" && bytes.Contains(saved, []byte("too many errors")) == mode.all {
						t.Fatalf("error limit not respected: %s", saved)
					}
					if os.Getenv("GO_SSA_NATIVE_EXECUTION") != "" {
						archive := filepath.Join(t.TempDir(), "native.a")
						args := []string{"tool", "compile", "-p=p", "-o", archive}
						if req.Options.CompilingRuntime {
							args = append(args, "-+")
						}
						if mode.all {
							args = append(args, "-e")
						}
						if mode.noCol {
							args = append(args, "-C")
						}
						if mode.actual {
							args = append(args, "-L")
						}
						args = append(args, file)
						cmd := exec.Command("go", args...)
						cmd.Env = append(os.Environ(), "GOTOOLCHAIN=go1.27.1", "GOWORK=off", "GOFLAGS=", "GOOS=linux", "GOARCH="+arch, "GOAMD64=v1", "GOARM64=v8.0", "GOEXPERIMENT=", "GOCOMPILEDEBUG=", "GOSSAFUNC=", "GOSSADIR=")
						output, nativeErr := cmd.CombinedOutput()
						if nativeErr == nil {
							t.Fatal("official accepted invalid source")
						}
						if !bytes.Equal(output, saved) {
							t.Fatalf("error report differs\nofficial:\n%s\nlibrary:\n%s", output, saved)
						}
					}
					next, e := compiler.Compile(context.Background(), request("p", "package p;func Good()int{return 42}"))
					if e != nil || len(next.Diagnostics) != 0 || !bytes.Equal(saved, failure.Report) {
						t.Fatal("failed compilation leaked state", e)
					}
					if !strings.HasSuffix(string(failure.Report), "\n") {
						t.Fatal("missing native newline")
					}
				})
			}
		}
	}
}

func TestNativeMultiFileErrors(t *testing.T) {
	for _, example := range []struct {
		name    string
		sources []string
	}{
		{"package-fatal", []string{"not a package\n", "package p\nfunc F( {\n"}},
		{"syntax", []string{"package p\nfunc F( {\n", "package p\nfunc G( {\n"}},
		{"pragma-types", []string{"package p\n//go:systemstack\nfunc F(){}\n", "package p\nvar A int = true\n"}},
	} {
		t.Run(example.name, func(t *testing.T) {
			req := request("p", "")
			req.Files = nil
			req.Options.UnlimitedErrors = true
			var names []string
			for i, source := range example.sources {
				name := filepath.Join(t.TempDir(), fmt.Sprintf("file%d.go", i))
				if err := os.WriteFile(name, []byte(source), 0600); err != nil {
					t.Fatal(err)
				}
				names = append(names, name)
				req.Files = append(req.Files, compiler.SourceFile{Name: name, Data: []byte(source)})
			}
			result, err := compiler.Compile(context.Background(), req)
			var failure *compiler.Error
			if result != nil || !errors.As(err, &failure) {
				t.Fatalf("missing diagnostics: %v", err)
			}
			if os.Getenv("GO_SSA_NATIVE_EXECUTION") == "" {
				return
			}
			args := append([]string{"tool", "compile", "-p=p", "-e", "-o", filepath.Join(t.TempDir(), "native.a")}, names...)
			cmd := exec.Command("go", args...)
			cmd.Env = append(os.Environ(), "GOTOOLCHAIN=go1.27.1", "GOWORK=off", "GOFLAGS=", "GOOS=linux", "GOARCH=amd64", "GOAMD64=v1", "GOEXPERIMENT=", "GOCOMPILEDEBUG=", "GOSSAFUNC=", "GOSSADIR=")
			output, nativeErr := cmd.CombinedOutput()
			if nativeErr == nil {
				t.Fatal("official accepted invalid source")
			}
			if !bytes.Equal(output, failure.Report) {
				t.Fatalf("error report differs\nofficial:\n%s\nlibrary:\n%s", output, failure.Report)
			}
		})
	}
}

package compiler_test

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"go-backend-ssa/compiler"
)

func TestSpectreCompilerOracle(t *testing.T) {
	const source = `package p
 func Index(a []int,i int)int{return a[i]}
 func Slice(a []int,i,j int)[]int{return a[i:j]}
 func Call(f func(int)int, i int)int{return f(i)}
 `
	file := filepath.Join(t.TempDir(), "spectre.go")
	if err := os.WriteFile(file, []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	for _, arch := range []string{"amd64", "arm64"} {
		for _, mode := range []string{"", "index", "ret", "all", " index,ret,index ", "bad"} {
			t.Run(arch+"/"+mode, func(t *testing.T) {
				t.Parallel()
				req := request("p", source)
				req.Files[0].Name = file
				req.Target = compiler.Target{GOOS: "linux", GOARCH: arch, GOAMD64: 1}
				if arch == "arm64" {
					req.Target.GOAMD64 = 0
					req.Target.GOARM64 = "v8.0"
				}
				req.Options.Spectre = mode
				req.Options.AssemblyListing = 1
				result, err := compiler.Compile(context.Background(), req)
				invalid := mode == "bad" || arch == "arm64" && mode != ""
				if invalid {
					if result != nil || err == nil {
						t.Fatal("invalid setting accepted")
					}
				} else {
					if err != nil {
						t.Fatal(err)
					}
					if arch == "amd64" && (mode == "ret" || mode == "all") && !bytes.Contains(result.AssemblyListing, []byte("runtime.retpoline")) {
						t.Fatal("missing retpoline")
					}
					if arch == "amd64" && (mode == "index" || mode == "all") && !bytes.Contains(result.AssemblyListing, []byte("CMOVQ")) {
						t.Fatal("missing index mitigation")
					}
				}
				if os.Getenv("GO_SSA_NATIVE_EXECUTION") == "" {
					return
				}
				archive := filepath.Join(t.TempDir(), "native.a")
				cmd := exec.Command("go", "tool", "compile", "-p=p", "-S", "-spectre="+mode, "-o", archive, file)
				cmd.Env = append(os.Environ(), "GOTOOLCHAIN=go1.27.1", "GOWORK=off", "GOFLAGS=", "GOOS=linux", "GOARCH="+arch, "GOAMD64=v1", "GOARM64=v8.0", "GOEXPERIMENT=", "GOCOMPILEDEBUG=", "GOSSAFUNC=", "GOSSADIR=")
				output, nativeErr := cmd.CombinedOutput()
				if invalid {
					if nativeErr == nil || !strings.Contains(string(output), err.Error()) {
						t.Fatalf("error differs: library=%v official=%s", err, output)
					}
					return
				}
				if nativeErr != nil {
					t.Fatalf("official: %v\n%s", nativeErr, output)
				}
				native, err := os.ReadFile(archive)
				if err != nil {
					t.Fatal(err)
				}
				if !bytes.Equal(native, result.Archive) {
					t.Fatalf("archive differs: official=%d library=%d", len(native), len(result.Archive))
				}
				if !bytes.Equal(output, result.AssemblyListing) {
					t.Fatalf("assembly differs: official=%d library=%d", len(output), len(result.AssemblyListing))
				}
			})
		}
	}
}

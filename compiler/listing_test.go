package compiler_test

import (
	"bytes"
	"context"
	"fmt"
	"go-backend-ssa/compiler"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestAssemblyListing(t *testing.T) {
	const source = `package p
var Data = "assembly output"
//go:noinline
func F(x int) int { if x < 0 { return -x }; return x+len(Data) }
func Closure(x int) func(int)int { return func(y int)int{return F(x+y)} }
`
	file := filepath.Join(t.TempDir(), "listing.go")
	if err := os.WriteFile(file, []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	for _, arch := range []string{"amd64", "arm64"} {
		for _, level := range []int{0, 1, 2, 3} {
			for _, noopt := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/level-%d/noopt-%t", arch, level, noopt), func(t *testing.T) {
					t.Parallel()
					req := request("p", source)
					req.Files[0].Name = file
					req.Target = compiler.Target{GOOS: "linux", GOARCH: arch, GOAMD64: 1}
					if arch == "arm64" {
						req.Target.GOAMD64 = 0
						req.Target.GOARM64 = "v8.0"
					}
					req.Options = compiler.Options{AssemblyListing: level, DisableOptimization: noopt, Workers: 4}
					result, err := compiler.Compile(context.Background(), req)
					if err != nil {
						t.Fatal(err)
					}
					if level == 0 {
						if len(result.AssemblyListing) != 0 {
							t.Fatal("unsolicited assembly listing")
						}
					} else {
						for _, part := range []string{"p.F", "\tTEXT\t", "\tFUNCDATA\t", "\trel "} {
							if !strings.Contains(string(result.AssemblyListing), part) {
								t.Fatalf("missing %q in assembly listing", part)
							}
						}
					}
					if os.Getenv("GO_SSA_NATIVE_EXECUTION") == "" {
						return
					}
					archive := filepath.Join(t.TempDir(), "reference.a")
					args := []string{"tool", "compile", "-p=p", "-o", archive}
					if level != 0 {
						args = append(args, fmt.Sprintf("-S=%d", level))
					}
					if noopt {
						args = append(args, "-N")
					}
					args = append(args, file)
					cmd := exec.Command("go", args...)
					cmd.Env = append(os.Environ(), "GOTOOLCHAIN=go1.27.1", "GOWORK=off", "GOFLAGS=", "GOOS=linux", "GOARCH="+arch, "GOAMD64=v1", "GOARM64=v8.0", "GOEXPERIMENT=", "GOCOMPILEDEBUG=", "GOSSAFUNC=")
					listing, err := cmd.CombinedOutput()
					if err != nil {
						t.Fatalf("official compiler: %v\n%s", err, listing)
					}
					reference, err := os.ReadFile(archive)
					if err != nil {
						t.Fatal(err)
					}
					if !bytes.Equal(reference, result.Archive) {
						t.Fatalf("archive differs, official=%d library=%d", len(reference), len(result.Archive))
					}
					if !bytes.Equal(listing, result.AssemblyListing) {
						os.WriteFile(filepath.Join(t.TempDir(), "library.txt"), result.AssemblyListing, 0600)
						t.Fatalf("assembly listing differs, official=%d library=%d\nofficial:\n%s\nlibrary:\n%s", len(listing), len(result.AssemblyListing), listing, result.AssemblyListing)
					}
				})
			}
		}
	}
}

func TestAssemblyListingRequiresNativeCompile(t *testing.T) {
	req := request("p", "package p; func F(){}")
	req.Options.AssemblyListing = 1
	for _, stage := range []compiler.Stage{compiler.Built, compiler.EarlyOptimized} {
		req.Options.Stage = stage
		if result, err := compiler.Build(context.Background(), req); err == nil || result != nil {
			t.Fatal("SSA stage accepted native assembly listing")
		}
	}
	req.Options.Stage = ""
	req.Options.AssemblyListing = -1
	if result, err := compiler.Compile(context.Background(), req); err == nil || result != nil {
		t.Fatal("negative assembly listing level accepted")
	}
}

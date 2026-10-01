package compiler_test

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"go-backend-ssa/compiler"
)

func TestLinkModesCompilerOracle(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "p.go")
	if err := os.WriteFile(file, []byte(sharedSource), 0600); err != nil {
		t.Fatal(err)
	}
	for _, arch := range []string{"amd64", "arm64"} {
		for _, dynamic := range []bool{false, true} {
			for _, split := range []bool{false, true} {
				for _, noopt := range []bool{false, true} {
					t.Run(fmt.Sprintf("%s/dynamic=%t/split=%t/noopt=%t", arch, dynamic, split, noopt), func(t *testing.T) {
						t.Parallel()
						req := request("example/shared", sharedSource)
						req.Files[0].Name = file
						req.Options = compiler.Options{DynamicLink: dynamic, SplitObjects: split, AssemblyListing: 1, DisableOptimization: noopt, DisableInlining: noopt}
						if arch == "arm64" {
							req.Target = compiler.Target{GOOS: "linux", GOARCH: arch, GOARM64: "v8.0"}
						}
						result, err := compiler.Compile(context.Background(), req)
						if err != nil {
							t.Fatal(err)
						}
						if (len(result.LinkerArchive) != 0) != split {
							t.Fatal("incorrect linker archive presence")
						}
						if split {
							if bytes.Contains(result.Archive, []byte("_go_.o")) || !bytes.Contains(result.Archive, []byte("__.PKGDEF")) {
								t.Fatal("compiler archive members incorrect")
							}
							if !bytes.Contains(result.LinkerArchive, []byte("_go_.o")) || bytes.Contains(result.LinkerArchive, []byte("__.PKGDEF")) {
								t.Fatal("linker archive members incorrect")
							}
							// Compiler-only exports must remain usable by the native importer.
							user := request("example/user", `package user;import p "example/shared";func F()int{return p.New(7).N}`)
							user.Target = req.Target
							user.Options = req.Options
							user.Imports = map[string][]byte{req.ImportPath: result.Archive}
							if _, err := compiler.Compile(context.Background(), user); err != nil {
								t.Fatal(err)
							}
						}
						if os.Getenv("GO_SSA_NATIVE_EXECUTION") == "" {
							return
						}
						archive := filepath.Join(t.TempDir(), "native.a")
						linker := archive + ".lo"
						args := []string{"tool", "compile", "-p", req.ImportPath, "-S", "-o", archive}
						if dynamic {
							args = append(args, "-dynlink")
						}
						if split {
							args = append(args, "-linkobj", linker)
						}
						if noopt {
							args = append(args, "-N", "-l")
						}
						args = append(args, file)
						cmd := exec.Command("go", args...)
						cmd.Env = append(os.Environ(), "GOTOOLCHAIN=go1.27.1", "GOWORK=off", "GOFLAGS=", "GOOS=linux", "GOARCH="+arch, "GOAMD64=v1", "GOARM64=v8.0", "GOEXPERIMENT=", "GOCOMPILEDEBUG=", "GOSSAFUNC=", "GOSSADIR=")
						output, err := cmd.CombinedOutput()
						if err != nil {
							t.Fatalf("official: %v\n%s", err, output)
						}
						native, err := os.ReadFile(archive)
						if err != nil {
							t.Fatal(err)
						}
						if !bytes.Equal(native, result.Archive) {
							t.Fatalf("compiler archive differs: official=%d library=%d", len(native), len(result.Archive))
						}
						if !bytes.Equal(output, result.AssemblyListing) {
							t.Fatalf("assembly differs: official=%d library=%d", len(output), len(result.AssemblyListing))
						}
						if split {
							native, err = os.ReadFile(linker)
							if err != nil {
								t.Fatal(err)
							}
							if !bytes.Equal(native, result.LinkerArchive) {
								t.Fatalf("linker archive differs: official=%d library=%d", len(native), len(result.LinkerArchive))
							}
						}
					})
				}
			}
		}
	}
}

func TestDynamicLinkStagesAndArtifacts(t *testing.T) {
	for _, arch := range []string{"amd64", "arm64"} {
		for _, stage := range []compiler.Stage{compiler.Built, compiler.EarlyOptimized} {
			t.Run(arch+"/"+string(stage), func(t *testing.T) {
				t.Parallel()
				req := request("example/shared", sharedSource)
				req.Options.DynamicLink = true
				req.Options.Stage = stage
				if arch == "arm64" {
					req.Target = compiler.Target{GOOS: "linux", GOARCH: arch, GOARM64: "v8.0"}
				}
				p, err := compiler.Build(context.Background(), req)
				if err != nil {
					t.Fatal(err)
				}
				if !p.Metadata.DynamicLink || !p.Metadata.Shared {
					t.Fatal("missing dynamic/shared identity")
				}
				saved := builtText(p)
				encoded := p.Artifact.Bytes()
				artifact, err := compiler.DecodeArtifact(encoded)
				if err != nil {
					t.Fatal(err)
				}
				user := request("example/user", `package user;import p "example/shared";func F()int{return p.New(7).N}`)
				user.Target = req.Target
				user.Options = req.Options
				user.Artifacts = map[string]*compiler.Artifact{req.ImportPath: artifact}
				if _, err := compiler.Build(context.Background(), user); err != nil {
					t.Fatal(err)
				}
				user.Options.Shared = true
				if _, err := compiler.Build(context.Background(), user); err != nil {
					t.Fatal("explicit shared changed effective mode", err)
				}
				user.Options.DynamicLink = false
				if _, err := compiler.Build(context.Background(), user); err == nil {
					t.Fatal("non-dynamic shared artifact accepted")
				}
				req.Options.DynamicLink = false
				plain, err := compiler.Build(context.Background(), req)
				if err != nil {
					t.Fatal(err)
				}
				if plain.Metadata.Shared || plain.Metadata.DynamicLink || builtText(p) != saved || !bytes.Equal(encoded, p.Artifact.Bytes()) {
					t.Fatal("dynamic state leaked")
				}
			})
		}
	}
}

func TestSplitObjectsRequiresCompile(t *testing.T) {
	req := request("p", "package p")
	req.Options.SplitObjects = true
	if p, err := compiler.Build(context.Background(), req); p != nil || err == nil {
		t.Fatal("split objects accepted by Build")
	}
}

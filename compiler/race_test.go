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

const raceSource = `package p
import "unsafe"
var V int
func F(p *int) int { V += *p; return V }
//go:norace
func Quiet(p *int) int { return *p }
func Pointer(p unsafe.Pointer,n uintptr) *int { return (*int)(unsafe.Pointer(uintptr(p)+n)) }
`

func TestRaceStagesAndArtifacts(t *testing.T) {
	for _, arch := range []string{"amd64", "arm64"} {
		for _, stage := range []compiler.Stage{compiler.Built, compiler.EarlyOptimized} {
			t.Run(arch+"/"+string(stage), func(t *testing.T) {
				t.Parallel()
				r := request("example/race", raceSource)
				r.Options.Race = true
				r.Options.Stage = stage
				r.Options.Workers = 4
				if arch == "arm64" {
					r.Target = compiler.Target{GOOS: "linux", GOARCH: arch, GOARM64: "v8.0"}
				}
				p, err := compiler.Build(context.Background(), r)
				if err != nil {
					t.Fatal(err)
				}
				saved := builtText(p)
				artifact := p.Artifact.Bytes()
				if !p.Metadata.Race || !strings.Contains(saved, "runtime.raceread") || !strings.Contains(saved, "runtime.checkptr") {
					t.Fatal("missing race or checkptr instrumentation", saved)
				}
				decoded, err := compiler.DecodeArtifact(artifact)
				if err != nil {
					t.Fatal(err)
				}
				consumer := request("example/user", `package user;import p "example/race";func G(v *int)int{return p.F(v)}`)
				consumer.Target = r.Target
				consumer.Options = r.Options
				consumer.Artifacts = map[string]*compiler.Artifact{"example/race": decoded}
				if _, err := compiler.Build(context.Background(), consumer); err != nil {
					t.Fatal(err)
				}
				consumer.Options.Race = false
				if _, err := compiler.Build(context.Background(), consumer); err == nil {
					t.Fatal("mixed instrumentation artifact accepted")
				}
				r.Options.Race = false
				plain, err := compiler.Build(context.Background(), r)
				if err != nil {
					t.Fatal(err)
				}
				if plain.Metadata.Race || strings.Contains(builtText(plain), "runtime.raceread") || builtText(p) != saved || !bytes.Equal(artifact, p.Artifact.Bytes()) {
					t.Fatal("race state leaked across instances")
				}
			})
		}
	}
}

func TestRaceOfficialArchives(t *testing.T) {
	if os.Getenv("GO_SSA_NATIVE_EXECUTION") == "" {
		t.Skip("set GO_SSA_NATIVE_EXECUTION for native race archive comparison")
	}
	dir := t.TempDir()
	file := filepath.Join(dir, "p.go")
	if err := os.WriteFile(file, []byte(raceSource), 0600); err != nil {
		t.Fatal(err)
	}
	for _, arch := range []string{"amd64", "arm64"} {
		for _, pkg := range []string{"example/race", "sync", "internal/runtime/atomic", "runtime/race"} {
			for _, noopt := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/%s/noopt-%t", arch, pkg, noopt), func(t *testing.T) {
					r := request(pkg, raceSource)
					r.Files[0].Name = file
					r.Options = compiler.Options{Race: true, Workers: 4, Standard: pkg != "example/race", DisableOptimization: noopt, DisableInlining: noopt}
					if arch == "arm64" {
						r.Target = compiler.Target{GOOS: "linux", GOARCH: arch, GOARM64: "v8.0"}
					}
					archive := filepath.Join(dir, "reference.a")
					args := []string{"tool", "compile", "-race", "-c=4", "-p", pkg, "-o", archive}
					if r.Options.Standard {
						args = append(args, "-std")
					}
					if noopt {
						args = append(args, "-N", "-l")
					}
					args = append(args, file)
					cmd := exec.Command("go", args...)
					cmd.Dir = dir
					cmd.Env = append(os.Environ(), "GOTOOLCHAIN=go1.27.1", "GOWORK=off", "GOFLAGS=", "GOOS=linux", "GOARCH="+arch, "GOAMD64=v1", "GOARM64=v8.0", "GOEXPERIMENT=", "GOCOMPILEDEBUG=", "GOSSAFUNC=")
					if out, err := cmd.CombinedOutput(); err != nil {
						t.Fatalf("official: %v\n%s", err, out)
					}
					ref, err := os.ReadFile(archive)
					if err != nil {
						t.Fatal(err)
					}
					actual, err := compiler.Compile(context.Background(), r)
					if err != nil {
						t.Fatal(err)
					}
					if !bytes.Equal(ref, actual.Archive) {
						t.Fatalf("full archive differs: official=%d library=%d", len(ref), len(actual.Archive))
					}
				})
			}
		}
	}
}

package compiler_test

import (
	"bytes"
	"context"
	"fmt"
	"go-backend-ssa/compiler"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

const sharedSource = `package p
type T struct{P *T;N int}
var G=T{N:3}
func New(n int)*T{return &T{P:&G,N:n}}
func F()func()int{return func()int{return G.N}}
`

func TestSharedStagesAndArtifacts(t *testing.T) {
	for _, arch := range []string{"amd64", "arm64"} {
		for _, stage := range []compiler.Stage{compiler.Built, compiler.EarlyOptimized} {
			t.Run(arch+"/"+string(stage), func(t *testing.T) {
				t.Parallel()
				r := request("example/shared", sharedSource)
				r.Options.Shared = true
				r.Options.Stage = stage
				if arch == "arm64" {
					r.Target = compiler.Target{GOOS: "linux", GOARCH: arch, GOARM64: "v8.0"}
				}
				p, err := compiler.Build(context.Background(), r)
				if err != nil {
					t.Fatal(err)
				}
				if !p.Metadata.Shared {
					t.Fatal("missing shared-code identity")
				}
				saved := builtText(p)
				encoded := p.Artifact.Bytes()
				artifact, err := compiler.DecodeArtifact(encoded)
				if err != nil {
					t.Fatal(err)
				}
				user := request("example/user", `package user;import p "example/shared";func F()int{return p.New(7).N}`)
				user.Options = r.Options
				user.Target = r.Target
				user.Artifacts = map[string]*compiler.Artifact{"example/shared": artifact}
				if _, err := compiler.Build(context.Background(), user); err != nil {
					t.Fatal(err)
				}
				user.Options.Shared = false
				if _, err := compiler.Build(context.Background(), user); err == nil {
					t.Fatal("mixed shared-code artifact accepted")
				}
				r.Options.Shared = false
				plain, err := compiler.Build(context.Background(), r)
				if err != nil {
					t.Fatal(err)
				}
				if plain.Metadata.Shared || builtText(p) != saved || !bytes.Equal(encoded, p.Artifact.Bytes()) {
					t.Fatal("shared-code state leaked")
				}
			})
		}
	}
}

func TestSharedOfficialArchives(t *testing.T) {
	if os.Getenv("GO_SSA_NATIVE_EXECUTION") == "" {
		t.Skip("set GO_SSA_NATIVE_EXECUTION for native shared archive comparison")
	}
	dir := t.TempDir()
	file := filepath.Join(dir, "p.go")
	if err := os.WriteFile(file, []byte(sharedSource), 0600); err != nil {
		t.Fatal(err)
	}
	for _, arch := range []string{"amd64", "arm64"} {
		for _, mode := range []string{"shared", "shared-noopt", "shared-race"} {
			t.Run(arch+"/"+mode, func(t *testing.T) {
				r := request("example/shared", sharedSource)
				r.Files[0].Name = file
				r.Options.Shared = true
				r.Options.Workers = 4
				if arch == "arm64" {
					r.Target = compiler.Target{GOOS: "linux", GOARCH: arch, GOARM64: "v8.0"}
				}
				archive := filepath.Join(dir, "reference.a")
				args := []string{"tool", "compile", "-shared", "-c=4", "-p", r.ImportPath, "-o", archive}
				switch mode {
				case "shared-noopt":
					r.Options.DisableOptimization = true
					r.Options.DisableInlining = true
					args = append(args, "-N", "-l")
				case "shared-race":
					r.Options.Race = true
					args = append(args, "-race")
				}
				args = append(args, file)
				cmd := exec.Command("go", args...)
				cmd.Dir = dir
				cmd.Env = append(os.Environ(), "GOTOOLCHAIN=go1.27.1", "GOWORK=off", "GOFLAGS=", "GOOS=linux", "GOARCH="+arch, "GOAMD64=v1", "GOARM64=v8.0", "GOEXPERIMENT=", "GOCOMPILEDEBUG=", "GOSSAFUNC=")
				if out, err := cmd.CombinedOutput(); err != nil {
					t.Fatalf("official: %v\n%s", err, out)
				}
				reference, err := os.ReadFile(archive)
				if err != nil {
					t.Fatal(err)
				}
				result, err := compiler.Compile(context.Background(), r)
				if err != nil {
					t.Fatal(err)
				}
				if !bytes.Equal(reference, result.Archive) {
					t.Fatal(fmt.Sprintf("full shared archive differs: official=%d library=%d", len(reference), len(result.Archive)))
				}
			})
		}
	}
}

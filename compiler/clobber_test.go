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

func TestClobberCompilerOracle(t *testing.T) {
	const source = `package p
 type T [2]*int
 var A,B T
 //go:noinline
 func Use(T) {}
 func F() { x,y:=A,B; Use(x); Use(y) }
 func Arithmetic(a,b,c int)int { x:=a*b; Use(A); return x+c }
 `
	file := filepath.Join(t.TempDir(), "clobber.go")
	if err := os.WriteFile(file, []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	for _, arch := range []string{"amd64", "arm64"} {
		for _, mode := range []struct {
			name      string
			dead, reg bool
			hash      string
		}{
			{"none", false, false, ""}, {"stack", true, false, ""}, {"register", false, true, ""}, {"both", true, true, ""},
			{"hash0", true, false, "0"}, {"hash1", true, false, "1"},
		} {
			t.Run(arch+"/"+mode.name, func(t *testing.T) {
				t.Parallel()
				req := request("p", source)
				req.Files[0].Name = file
				req.Target = compiler.Target{GOOS: "linux", GOARCH: arch, GOAMD64: 1}
				if arch == "arm64" {
					req.Target.GOAMD64 = 0
					req.Target.GOARM64 = "v8.0"
				}
				req.Options.ClobberDead, req.Options.ClobberDeadReg = mode.dead, mode.reg
				req.Options.AssemblyListing = 1
				req.Debug.ClobberDeadHash = mode.hash
				result, err := compiler.Compile(context.Background(), req)
				if err != nil {
					t.Fatal(err)
				}
				if mode.dead && mode.hash == "" && !bytes.Contains(result.AssemblyListing, []byte("3735936685")) {
					t.Fatal("missing native stack clobber instructions")
				}
				if os.Getenv("GO_SSA_NATIVE_EXECUTION") == "" {
					return
				}
				archive := filepath.Join(t.TempDir(), "native.a")
				args := []string{"tool", "compile", "-p=p", "-S", "-o", archive, fmt.Sprintf("-clobberdead=%t", mode.dead), fmt.Sprintf("-clobberdeadreg=%t", mode.reg), file}
				cmd := exec.Command("go", args...)
				cmd.Env = append(os.Environ(), "GOTOOLCHAIN=go1.27.1", "GOWORK=off", "GOFLAGS=", "GOOS=linux", "GOARCH="+arch, "GOAMD64=v1", "GOARM64=v8.0", "GOEXPERIMENT=", "GOCOMPILEDEBUG=", "GOSSAFUNC=", "GOSSADIR=", "GOCLOBBERDEADHASH="+mode.hash)
				output, err := cmd.CombinedOutput()
				if err != nil {
					t.Fatalf("official: %v\n%s", err, output)
				}
				native, err := os.ReadFile(archive)
				if err != nil {
					t.Fatal(err)
				}
				if !bytes.Equal(native, result.Archive) {
					t.Fatalf("archive differs: official=%d library=%d", len(native), len(result.Archive))
				}
				got := append(bytes.Clone(result.DebugOutput), result.AssemblyListing...)
				if !bytes.Equal(output, got) {
					t.Fatalf("output differs: official=%d library=%d\nofficial:\n%s\nlibrary:\n%s", len(output), len(got), output, got)
				}
			})
		}
	}
}

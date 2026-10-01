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

const rangeFuncDebugSource = `package p
func Sequence(yield func(int)bool){for i:=0;i<3;i++{if !yield(i){return}}}
func Sum(seed int)int{sum:=seed;for value:=range Sequence{sum+=value};return sum}
`

func TestRangeFuncNoOptDebug(t *testing.T) {
	file := filepath.Join(t.TempDir(), "rangefunc.go")
	if err := os.WriteFile(file, []byte(rangeFuncDebugSource), 0600); err != nil {
		t.Fatal(err)
	}

	for _, arch := range []string{"amd64", "arm64"} {
		for _, race := range []bool{false, true} {
			for _, nodwarf := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/race-%t/nodwarf-%t", arch, race, nodwarf), func(t *testing.T) {
					defer func() {
						if failure := recover(); failure != nil {
							t.Fatalf("rangefunc debug compilation panicked: %v", failure)
						}
					}()
					req := request("p", rangeFuncDebugSource)
					req.Files[0].Name = file
					req.Options = compiler.Options{DisableOptimization: true, DisableInlining: true, Race: race, DisableDwarf: nodwarf}
					if arch == "arm64" {
						req.Target = compiler.Target{GOOS: "linux", GOARCH: arch, GOARM64: "v8.0"}
					}
					var reference []byte
					if os.Getenv("GO_SSA_NATIVE_EXECUTION") != "" {
						archive := filepath.Join(t.TempDir(), "reference.a")
						args := []string{"tool", "compile", "-N", "-l", "-p", "p", "-o", archive}
						if race {
							args = append(args, "-race")
						}
						if nodwarf {
							args = append(args, "-dwarf=false")
						}
						args = append(args, file)
						cmd := exec.Command("go", args...)
						cmd.Env = append(os.Environ(), "GOTOOLCHAIN=go1.27.1", "GOWORK=off", "GOFLAGS=", "GOOS=linux", "GOARCH="+arch, "GOAMD64=v1", "GOARM64=v8.0", "GOEXPERIMENT=", "GOCOMPILEDEBUG=", "GOSSAFUNC=")
						if out, err := cmd.CombinedOutput(); err != nil {
							t.Fatalf("official: %v\n%s", err, out)
						}
						var err error
						reference, err = os.ReadFile(archive)
						if err != nil {
							t.Fatal(err)
						}
					}
					result, err := compiler.Compile(context.Background(), req)
					if err != nil || result == nil {
						t.Fatal("rangefunc compilation failed", err)
					}
					if reference != nil && !bytes.Equal(reference, result.Archive) {
						t.Fatalf("rangefunc archive differs: official=%d library=%d", len(reference), len(result.Archive))
					}
				})
			}
		}
	}
}

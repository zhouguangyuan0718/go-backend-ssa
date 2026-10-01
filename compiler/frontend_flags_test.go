package compiler_test

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"go-backend-ssa/compiler"
)

func TestFrontendFlagsCompilerOracle(t *testing.T) {
	const source = `package p
var Root *int
func Store(p *int) { Root=p }
func Add(x int) int { return x+1 }
func Index(p *int, a []int, i int) int { return *p+Add(a[i]) }
//go:noescape
func Touch(*[80000]byte)
func Frame() byte { var a [80000]byte; Touch(&a); return a[0] }
`
	file := filepath.Join(t.TempDir(), "frontend.go")
	if err := os.WriteFile(file, []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	modes := []struct {
		name string
		opts compiler.Options
		args []string
	}{
		{"default", compiler.Options{}, nil},
		{"inline-off-alias", compiler.Options{DisableInlining: true}, []string{"-l=1"}},
		{"inline-1", compiler.Options{InliningLevel: 1}, []string{"-l=1"}},
		{"inline-2", compiler.Options{InliningLevel: 2}, []string{"-l=2"}},
		{"inline-3", compiler.Options{InliningLevel: 3}, []string{"-l=3"}},
		{"inline-4", compiler.Options{InliningLevel: 4}, []string{"-l=4"}},
		{"inline-negative", compiler.Options{InliningLevel: -1}, []string{"-l=-1"}},
		{"bounds-1", compiler.Options{BoundsCheckMode: 1}, []string{"-B=1"}},
		{"bounds-2", compiler.Options{BoundsCheckMode: 2}, []string{"-B=2"}},
		{"bounds-negative", compiler.Options{BoundsCheckMode: -1}, []string{"-B=-1"}},
		{"live-1", compiler.Options{LivenessReport: 1, Workers: 4}, []string{"-live=1", "-c=4"}},
		{"live-2", compiler.Options{LivenessReport: 2, Workers: 4}, []string{"-live=2", "-c=4"}},
		{"no-write-barrier", compiler.Options{DisableWriteBarrier: true}, []string{"-wb=false"}},
		{"smallframes", compiler.Options{SmallFrames: true}, []string{"-smallframes"}},
		{"combined", compiler.Options{InliningLevel: 4, BoundsCheckMode: 1, LivenessReport: 2, SmallFrames: true}, []string{"-l=4", "-B", "-live=2", "-smallframes"}},
	}
	for _, arch := range []string{"amd64", "arm64"} {
		for _, mode := range modes {
			t.Run(arch+"/"+mode.name, func(t *testing.T) {
				t.Parallel()
				req := request("p", source)
				req.Files[0].Name = file
				req.Options = mode.opts
				req.Target = compiler.Target{GOOS: "linux", GOARCH: arch, GOAMD64: 1}
				if arch == "arm64" {
					req.Target.GOAMD64 = 0
					req.Target.GOARM64 = "v8.0"
				}
				result, err := compiler.Compile(context.Background(), req)
				if err != nil {
					t.Fatal(err)
				}
				if mode.opts.LivenessReport > 0 && len(result.DebugOutput) == 0 {
					t.Fatal("missing liveness report")
				}
				if os.Getenv("GO_SSA_NATIVE_EXECUTION") == "" {
					return
				}
				archive := filepath.Join(t.TempDir(), "native.a")
				args := append([]string{"tool", "compile", "-p=p", "-o", archive}, mode.args...)
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
					t.Fatalf("archive differs: official=%d library=%d", len(native), len(result.Archive))
				}
				if !bytes.Equal(output, result.DebugOutput) {
					t.Fatalf("report differs:\nofficial=%s\nlibrary=%s", output, result.DebugOutput)
				}
			})
		}
	}
}
func TestFrontendFlagsRejectAmbiguousInlining(t *testing.T) {
	req := request("p", "package p")
	req.Options = compiler.Options{DisableInlining: true, InliningLevel: 1}
	if result, err := compiler.Compile(context.Background(), req); result != nil || err == nil {
		t.Fatal("ambiguous options accepted")
	}
}

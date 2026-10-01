package compiler_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"go-backend-ssa/compiler"
)

func TestLazyImportSnapshots(t *testing.T) {
	for _, arch := range []string{"amd64", "arm64"} {
		for _, stage := range []compiler.Stage{"", compiler.Built, compiler.EarlyOptimized} {
			t.Run(arch+"/"+string(stage), func(t *testing.T) {
				t.Parallel()
				depReq := request("dep", "package dep;type Box[T any]struct{V T};var Seed=3;func Sum[T ~int](x T)T{return x+T(Seed)};func(b Box[T])Get()T{return b.V}")
				if arch == "arm64" {
					depReq.Target = compiler.Target{GOOS: "linux", GOARCH: arch, GOARM64: "v8.0"}
				}
				dep, err := compiler.Compile(context.Background(), depReq)
				if err != nil {
					t.Fatal(err)
				}
				req := request("use", `package use;import "dep";func F(x int)int{return dep.Sum(dep.Box[int]{V:x}.Get())}`)
				req.Target = depReq.Target
				req.Options.Stage = stage
				req.Imports = map[string][]byte{"dep": dep.Archive}
				var eager []byte
				if stage == "" {
					p, e := compiler.Compile(context.Background(), req)
					if e != nil {
						t.Fatal(e)
					}
					eager = p.Archive
				} else {
					p, e := compiler.Build(context.Background(), req)
					if e != nil {
						t.Fatal(e)
					}
					eager = p.Artifact.Bytes()
				}
				providerData := bytes.Clone(dep.Archive)
				var calls, unused atomic.Int32
				req.Imports = nil
				req.ImportLoaders = map[string]compiler.ImportLoader{
					"dep": func(context.Context) ([]byte, error) { calls.Add(1); return providerData, nil },
					"future-output": func(context.Context) ([]byte, error) {
						unused.Add(1)
						return nil, errors.New("archive has not been created yet")
					},
				}
				var lazy []byte
				var retained *compiler.Package
				if stage == "" {
					p, e := compiler.Compile(context.Background(), req)
					if e != nil {
						t.Fatal(e)
					}
					lazy = p.Archive
				} else {
					p, e := compiler.Build(context.Background(), req)
					if e != nil {
						t.Fatal(e)
					}
					lazy = p.Artifact.Bytes()
					retained = p
					if len(p.Artifact.Info().Dependencies) != 1 || p.Artifact.Info().Dependencies[0].SHA256 != sha256.Sum256(dep.Archive) {
						t.Fatal("lazy dependency identity missing")
					}
				}
				if !bytes.Equal(lazy, eager) || calls.Load() != 1 || unused.Load() != 0 {
					t.Fatalf("lazy import mismatch: calls=%d unused=%d eager=%d lazy=%d", calls.Load(), unused.Load(), len(eager), len(lazy))
				}
				if !bytes.Equal(providerData, dep.Archive) || req.Imports != nil || len(req.ImportLoaders) != 2 {
					t.Fatal("provider input mutated")
				}
				clear(providerData)
				if retained != nil && !bytes.Equal(retained.Artifact.Bytes(), eager) {
					t.Fatal("artifact aliases provider memory")
				}
			})
		}
	}
}

func TestLazyImportFailures(t *testing.T) {
	req := request("p", `package p;import "dep";var A=dep.F()`)
	sentinel := errors.New("archive reader failed")
	calls := 0
	req.ImportLoaders = map[string]compiler.ImportLoader{"dep": func(context.Context) ([]byte, error) { calls++; return nil, sentinel }}
	result, err := compiler.Compile(context.Background(), req)
	var sourceError *compiler.Error
	if result != nil || !errors.As(err, &sourceError) || !errors.Is(err, sentinel) || calls != 1 {
		t.Fatalf("provider failure lost: calls=%d error=%v", calls, err)
	}
	for _, mode := range []string{"nil", "duplicate", "artifact"} {
		t.Run(mode, func(t *testing.T) {
			bad := request("p", "package p")
			bad.ImportLoaders = map[string]compiler.ImportLoader{"dep": func(context.Context) ([]byte, error) {
				t.Fatal("invalid declaration invoked provider")
				return nil, nil
			}}
			switch mode {
			case "nil":
				bad.ImportLoaders["dep"] = nil
			case "duplicate":
				bad.Imports = map[string][]byte{"dep": nil}
			case "artifact":
				bad.Artifacts = map[string]*compiler.Artifact{"dep": nil}
			}
			if p, e := compiler.Compile(context.Background(), bad); p != nil || e == nil {
				t.Fatal("invalid import inputs accepted")
			}
		})
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req.ImportLoaders["dep"] = func(got context.Context) ([]byte, error) {
		if got != ctx {
			t.Fatal("wrong provider context")
		}
		cancel()
		return []byte("discarded"), nil
	}
	if p, e := compiler.Compile(ctx, req); p != nil || !errors.Is(e, context.Canceled) {
		t.Fatalf("cancellation became source error: %v", e)
	}
	marker := new(int)
	req.ImportLoaders["dep"] = func(context.Context) ([]byte, error) { panic(marker) }
	func() {
		defer func() {
			if recover() != marker {
				t.Error("provider panic identity lost")
			}
		}()
		compiler.Compile(context.Background(), req)
		t.Error("provider panic accepted")
	}()
	if _, e := compiler.Compile(context.Background(), request("p", "package p;func Good()int{return 1}")); e != nil {
		t.Fatal("failed provider poisoned compilation", e)
	}
}

func TestLazyArtifactClosure(t *testing.T) {
	dep, err := compiler.Compile(context.Background(), request("dep", "package dep;func F()int{return 7}"))
	if err != nil {
		t.Fatal(err)
	}
	midReq := request("mid", `package mid;import "dep";func F()int{return dep.F()}`)
	midReq.Imports = map[string][]byte{"dep": dep.Archive}
	mid := buildArtifact(t, midReq)
	root := request("root", `package root;import "mid";func F()int{return mid.F()}`)
	root.Artifacts = map[string]*compiler.Artifact{"mid": mid.Artifact}
	root.Imports = midReq.Imports
	eager := buildArtifact(t, root).Artifact.Bytes()
	root.Imports = nil
	calls := 0
	root.ImportLoaders = map[string]compiler.ImportLoader{"dep": func(context.Context) ([]byte, error) { calls++; return dep.Archive, nil }}
	if got := buildArtifact(t, root); !bytes.Equal(got.Artifact.Bytes(), eager) || calls != 1 {
		t.Fatal("lazy artifact closure changed", calls)
	}
	other, err := compiler.Compile(context.Background(), request("dep", "package dep;func F()int{return 8}"))
	if err != nil {
		t.Fatal(err)
	}
	root.ImportLoaders["dep"] = func(context.Context) ([]byte, error) { return other.Archive, nil }
	if p, e := compiler.Build(context.Background(), root); p != nil || e == nil || !strings.Contains(e.Error(), "different identity") {
		t.Fatalf("stale lazy artifact dependency accepted: %v", e)
	}
	sentinel := errors.New("dependency unavailable")
	root.ImportLoaders["dep"] = func(context.Context) ([]byte, error) { return nil, sentinel }
	if p, e := compiler.Build(context.Background(), root); p != nil || !errors.Is(e, sentinel) {
		t.Fatal("artifact provider error lost", e)
	}
}

func TestLazyImportsIndependentCompilations(t *testing.T) {
	dep, err := compiler.Compile(context.Background(), request("dep", "package dep;func F()int{return 7}"))
	if err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int32
	req := request("p", `package p;import "dep";func F()int{return dep.F()}`)
	req.ImportLoaders = map[string]compiler.ImportLoader{"dep": func(context.Context) ([]byte, error) { calls.Add(1); return dep.Archive, nil }}
	var workers sync.WaitGroup
	for range 8 {
		workers.Go(func() {
			if _, err := compiler.Compile(context.Background(), req); err != nil {
				t.Error(err)
			}
		})
	}
	workers.Wait()
	if calls.Load() != 8 {
		t.Fatal("import cache crossed compilation boundaries", calls.Load())
	}
}

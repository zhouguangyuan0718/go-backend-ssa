package compiler_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"go-backend-ssa/compiler"
	"strings"
	"testing"
)

func TestARM64SourceStagesAndArtifacts(t *testing.T) {
	for _, cpu := range []string{"", "v8.0", "v8.0,crypto,lse,lse", "v8.1", "v9.5"} {
		t.Run(cpu, func(t *testing.T) {
			t.Parallel()
			r := request("example/arm", `package arm
 type V struct{N int; S string}
 func(v V)Get()int{return v.N}
 var I any=V{3,"arm"}
 var M=map[string]int{"x":4}
 func Add[T ~int](a,b T)T{return a+b}
 func F(p *int,n int)int{s:=0;for i:=range n{if i&1==0{s+=i}else{s-=i}};*p+=s;return Add(*p,M["x"])}
 func D(p *int)int{defer func(){*p++}();return *p}
 func Float(x,y float64)float64{return x*y+x/y}
 `)
			r.Target = compiler.Target{GOOS: "linux", GOARCH: "arm64", GOARM64: cpu}
			r.Options.Workers = 4
			native, err := compiler.Compile(context.Background(), r)
			if err != nil {
				t.Fatal(err)
			}
			canonical := cpu
			switch cpu {
			case "":
				canonical = "v8.0"
			case "v8.0,crypto,lse,lse":
				canonical = "v8.0,lse,crypto"
			case "v8.1", "v9.5":
				canonical = cpu + ",lse"
			}
			if native.Target.GOARM64 != canonical || !bytes.Contains(native.Archive, []byte("go object linux arm64 ")) || !bytes.Contains(native.Archive, []byte("GOARM64="+canonical)) {
				t.Fatal("wrong target header", native.Target)
			}
			var previous *compiler.Package
			for _, stage := range []compiler.Stage{compiler.Built, compiler.EarlyOptimized} {
				r.Options.Stage = stage
				p, err := compiler.Build(context.Background(), r)
				if err != nil {
					t.Fatal(err)
				}
				if p.Metadata.Target.GOARM64 != canonical || len(p.Functions) < 5 || len(p.Globals) == 0 {
					t.Fatal("incomplete ARM64 package")
				}
				if _, err := compiler.DecodeArtifact(p.Artifact.Bytes()); err != nil {
					t.Fatal(err)
				}
				if previous != nil && !bytes.Equal(previous.Artifact.UnifiedIR(), p.Artifact.UnifiedIR()) {
					t.Fatal("early SSA changed frontend payload")
				}
				previous = p
				for _, f := range p.Functions {
					if f.RegAlloc != nil || len(f.Frontend().Func().LSym.P) != 0 {
						t.Fatal("Build emitted native code")
					}
				}
				consumer := request("example/consumer", `package consumer;import "example/arm";func G()int{return arm.Add(4,5)}`)
				consumer.Target = r.Target
				consumer.Artifacts = map[string]*compiler.Artifact{"example/arm": p.Artifact}
				if _, err := compiler.Build(context.Background(), consumer); err != nil {
					t.Fatal(err)
				}
				consumer.Target = compiler.Target{GOOS: "linux", GOARCH: "amd64", GOAMD64: 1}
				if _, err := compiler.Build(context.Background(), consumer); err == nil {
					t.Fatal("cross-architecture artifact accepted")
				}
			}
			saved := bytes.Clone(native.Archive)
			r.Options.Stage = ""
			r.Options.DisableOptimization = true
			next, err := compiler.Compile(context.Background(), r)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(native.Archive, saved) || bytes.Equal(next.Archive, saved) {
				t.Fatal("retained archive or optimization policy")
			}
		})
	}
}

func TestARM64TargetValidation(t *testing.T) {
	for _, target := range []compiler.Target{
		{GOOS: "linux", GOARCH: "arm64", GOAMD64: 1},
		{GOOS: "linux", GOARCH: "arm64", GOARM64: "v7.0"},
		{GOOS: "linux", GOARCH: "arm64", GOARM64: "v9.6"},
		{GOOS: "linux", GOARCH: "arm64", GOARM64: "v8.0,unknown"},
		{GOOS: "linux", GOARCH: "amd64", GOAMD64: 1, GOARM64: "v8.0"},
	} {
		t.Run(fmt.Sprint(target), func(t *testing.T) {
			r := request("p", "package p")
			r.Target = target
			if result, err := compiler.Compile(context.Background(), r); result != nil || err == nil || !strings.Contains(err.Error(), "target") && !strings.Contains(err.Error(), "GOARM64") {
				t.Fatal(result, err)
			}
		})
	}
}

func TestARM64AndAMD64InterleavedLifetimes(t *testing.T) {
	for _, stage := range []compiler.Stage{compiler.Built, compiler.EarlyOptimized} {
		t.Run(string(stage), func(t *testing.T) {
			t.Parallel()
			r := request("example/same", `package same;type T struct{N int;P *T};var V=T{N:7};func F(p *T,n int)int{for i:=range n{p.N+=i};return p.N}`)
			r.Options.Stage = stage
			r.Target = compiler.Target{GOOS: "linux", GOARCH: "arm64", GOARM64: "v8.1"}
			p, err := compiler.Build(context.Background(), r)
			if err != nil {
				t.Fatal(err)
			}
			retained := builtText(p)
			artifact := p.Artifact.Bytes()
			for _, target := range []compiler.Target{{GOOS: "linux", GOARCH: "amd64", GOAMD64: 3}, {GOOS: "linux", GOARCH: "arm64", GOARM64: "v8.0"}} {
				r.Target = target
				bad := r
				bad.Files = []compiler.SourceFile{{Name: "bad.go", Data: []byte("package same;func F(){missing()}")}}
				if q, err := compiler.Build(context.Background(), bad); q != nil || err == nil {
					t.Fatal("invalid build accepted")
				}
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				if q, err := compiler.Build(ctx, r); q != nil || !errors.Is(err, context.Canceled) {
					t.Fatal("cancelled build accepted", err)
				}
				q, err := compiler.Build(context.Background(), r)
				if err != nil {
					t.Fatal(err)
				}
				if q.Functions[0] == p.Functions[0] || q.Functions[0].Frontend().Func() == p.Functions[0].Frontend().Func() {
					t.Fatal("shared function graph")
				}
				if got := builtText(p); got != retained || !bytes.Equal(artifact, p.Artifact.Bytes()) {
					t.Fatal("another target mutated retained package")
				}
			}
			r.Target = compiler.Target{GOOS: "linux", GOARCH: "arm64", GOARM64: "v8.1"}
			q, err := compiler.Build(context.Background(), r)
			if err != nil {
				t.Fatal(err)
			}
			if builtText(q) != retained || !bytes.Equal(q.Artifact.Bytes(), artifact) {
				t.Fatal("target alternation changed output")
			}
		})
	}
}

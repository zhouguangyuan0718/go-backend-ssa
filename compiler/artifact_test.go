package compiler_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"go-backend-ssa/compiler"
	"go-backend-ssa/ir"
	"strings"
	"testing"
)

func buildArtifact(t *testing.T, req compiler.Request) *compiler.Package {
	t.Helper()
	p, err := compiler.Build(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if p.Artifact == nil {
		t.Fatal("Build did not produce an import artifact")
	}
	encoded := p.Artifact.Bytes()
	decoded, err := compiler.DecodeArtifact(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.SHA256() != sha256.Sum256(encoded) || !bytes.Equal(decoded.Bytes(), encoded) {
		t.Fatal("artifact identity")
	}
	raw := decoded.UnifiedIR()
	fingerprint := decoded.Fingerprint()
	if len(raw) < len(fingerprint) || !bytes.Equal(raw[len(raw)-len(fingerprint):], fingerprint[:]) {
		t.Fatal("native linker fingerprint")
	}
	clear(raw)
	if decoded.Fingerprint() != fingerprint {
		t.Fatal("raw payload aliases artifact storage")
	}
	clear(encoded)
	if len(decoded.Bytes()) == 0 || decoded.Info().ImportPath != req.ImportPath {
		t.Fatal("artifact retains caller buffer")
	}
	p.Artifact = decoded
	return p
}
func TestSourceArtifactRoundTrip(t *testing.T) {
	for i := range 4 {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			t.Parallel()
			leaf := buildArtifact(t, request("example/leaf", `package leaf
 type Value struct{X int; Label string}
 func(v Value)Get()int{return v.X}
 var Count=3
 func init(){Count++}
 func hidden(x int)int{return x+Count}
 func Add[T ~int](x,y T)T{return x+y}
 func Twice(x int)int{return hidden(x)+hidden(x)}
 `))
			midReq := request("example/mid", `package mid
 import "example/leaf"
 type Alias=leaf.Value
 var Initial=leaf.Twice(4)
 func Add[T ~int](x,y T)T{return leaf.Add(x,y)}
 func Use(v Alias)int{return v.Get()+Initial}
 `)
			midReq.Artifacts = map[string]*compiler.Artifact{"example/leaf": leaf.Artifact}
			mid := buildArtifact(t, midReq)
			info := mid.Artifact.Info()
			if info.NativeObjectReferences {
				t.Fatal("artifact-only build retained native object references")
			}
			if len(info.Dependencies) != 1 || info.Dependencies[0].Kind != "artifact" || info.Dependencies[0].SHA256 != leaf.Artifact.SHA256() {
				t.Fatal("dependency identity")
			}
			info.Dependencies[0].Path = "changed"
			if mid.Artifact.Info().Dependencies[0].Path != "example/leaf" {
				t.Fatal("metadata aliases artifact")
			}
			root := request("example/root", `package root
 import "example/leaf"
 import "example/mid"
 var A leaf.Value
 var B mid.Alias
 func F(x int)int{return mid.Add(x,7)+mid.Use(leaf.Value{X:x})}
 `)
			root.Artifacts = map[string]*compiler.Artifact{"example/leaf": leaf.Artifact, "example/mid": mid.Artifact}
			got := buildArtifact(t, root)
			a := got.Types.LocalPkg.Syms["A"].Def.(*ir.Name).Type()
			b := got.Types.LocalPkg.Syms["B"].Def.(*ir.Name).Type()
			if a != b || a.Universe() != got.Types || a.Universe() == leaf.Types || a.Universe() == mid.Types {
				t.Fatal("cross-package named type identity")
			}
			text := builtText(got)
			if !strings.Contains(text, "dict") {
				t.Fatal("generic dictionary missing")
			}
			if len(got.Imports) != 2 || got.Imports[0].ArtifactSHA256 != leaf.Artifact.SHA256() || got.Imports[0].Fingerprint != leaf.Artifact.Fingerprint() {
				t.Fatal("package artifact import metadata")
			}
			if repeat := buildArtifact(t, root); !bytes.Equal(repeat.Artifact.Bytes(), got.Artifact.Bytes()) || builtText(got) != text {
				t.Fatal("unstable artifact or retained graph")
			}
			// The native output entry point can consume the same name-based frontend
			// inputs. Machine code for dependencies is supplied separately at link time.
			if _, err := compiler.Compile(context.Background(), root); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestArtifactCompatibilityAndClosure(t *testing.T) {
	leaf := buildArtifact(t, request("leaf", "package leaf; type T int; func F()int{return 1}"))
	midReq := request("mid", `package mid;import "leaf";func F()int{return leaf.F()}`)
	midReq.Artifacts = map[string]*compiler.Artifact{"leaf": leaf.Artifact}
	mid := buildArtifact(t, midReq)
	valid := func() compiler.Request {
		r := request("p", `package p;import "mid";func F()int{return mid.F()}`)
		r.Artifacts = map[string]*compiler.Artifact{"leaf": leaf.Artifact, "mid": mid.Artifact}
		return r
	}
	replacement := buildArtifact(t, request("leaf", "package leaf; type T int; func F()int{return 2}"))
	native, err := compiler.Compile(context.Background(), request("mid", "package mid"))
	if err != nil {
		t.Fatal(err)
	}
	for name, change := range map[string]func(*compiler.Request){
		"missing dependency": func(r *compiler.Request) { delete(r.Artifacts, "leaf") },
		"changed dependency": func(r *compiler.Request) { r.Artifacts["leaf"] = replacement.Artifact },
		"ambiguous input":    func(r *compiler.Request) { r.Imports = map[string][]byte{"mid": native.Archive} },
		"wrong package":      func(r *compiler.Request) { r.Artifacts = map[string]*compiler.Artifact{"wrong": leaf.Artifact} },
		"zero artifact":      func(r *compiler.Request) { r.Artifacts["mid"] = new(compiler.Artifact) },
		"nil artifact":       func(r *compiler.Request) { r.Artifacts["mid"] = nil },
		"language":           func(r *compiler.Request) { r.Options.Language = "go1.26" },
		"target":             func(r *compiler.Request) { r.Target.GOAMD64 = 3 },
		"experiment":         func(r *compiler.Request) { r.Target.Experiments = "simd" },
	} {
		t.Run(name, func(t *testing.T) {
			r := valid()
			change(&r)
			if p, err := compiler.Build(context.Background(), r); p != nil || err == nil {
				t.Fatal("incompatible artifact accepted", p != nil, err)
			}
		})
	}
	if _, err := compiler.Build(context.Background(), valid()); err != nil {
		t.Fatal("failure poisoned next build", err)
	}
}

func TestArtifactNativeArchiveAdapter(t *testing.T) {
	req := request("dep", "package dep;type T struct{X int};func F(v T)int{return v.X+2}")
	native, err := compiler.Compile(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	midReq := request("mid", `package mid;import "dep";type T=dep.T;func F(v T)int{return dep.F(v)}`)
	midReq.Imports = map[string][]byte{"dep": native.Archive}
	mid := buildArtifact(t, midReq)
	if !mid.Artifact.Info().NativeObjectReferences {
		t.Fatal("native archive capability missing")
	}
	depInfo := mid.Artifact.Info().Dependencies
	if len(depInfo) != 1 || depInfo[0].Kind != "native-archive" || depInfo[0].SHA256 != sha256.Sum256(native.Archive) {
		t.Fatal("archive adapter identity")
	}
	root := request("root", `package root;import "mid";import "dep";func F()int{return mid.F(dep.T{X:7})}`)
	root.Artifacts = map[string]*compiler.Artifact{"mid": mid.Artifact}
	root.Imports = midReq.Imports
	if !buildArtifact(t, root).Artifact.Info().NativeObjectReferences {
		t.Fatal("transitive native archive capability missing")
	}
	delete(root.Imports, "dep")
	if p, err := compiler.Build(context.Background(), root); p != nil || err == nil {
		t.Fatal("missing native adapter dependency accepted")
	}
}

func TestArtifactMatchingLanguageAndTarget(t *testing.T) {
	for _, mode := range []string{"language", "cpu", "experiment", "canonical experiment"} {
		t.Run(mode, func(t *testing.T) {
			depReq := request("dep", "package dep;func F()int{return 7}")
			consumer := request("p", `package p;import "dep";func F()int{return dep.F()}`)
			switch mode {
			case "language":
				depReq.Options.Language = "go1.26"
				consumer.Options.Language = "go1.26"
			case "cpu":
				depReq.Target.GOAMD64 = 3
				consumer.Target.GOAMD64 = 3
			case "experiment":
				depReq.Target.Experiments = "simd"
				consumer.Target.Experiments = "simd"
			case "canonical experiment":
				depReq.Target.Experiments = "regabi"
			}
			dep := buildArtifact(t, depReq)
			consumer.Artifacts = map[string]*compiler.Artifact{"dep": dep.Artifact}
			buildArtifact(t, consumer)
		})
	}
}

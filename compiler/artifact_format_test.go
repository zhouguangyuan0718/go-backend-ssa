package compiler

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"testing"
)

func TestArtifactEnvelopeValidation(t *testing.T) {
	req := Request{ImportPath: "p", Files: []SourceFile{{Name: "p.go", Data: []byte("package p;type T struct{X int};func F[T ~int](x T)T{return x+1}")}}, Target: Target{GOOS: "linux", GOARCH: "amd64", GOAMD64: 1}}
	p, err := Build(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	good := p.Artifact.Bytes()
	if _, err := DecodeArtifact(good); err != nil {
		t.Fatal(err)
	}
	for _, n := range []int{0, 1, len(artifactMagic), len(artifactMagic) + 3, len(good) - 1} {
		if _, err := DecodeArtifact(good[:n]); err == nil {
			t.Fatal("truncation accepted", n)
		}
	}
	for i := 0; i < len(good); i += 17 {
		bad := bytes.Clone(good)
		bad[i] ^= 1
		if _, err := DecodeArtifact(bad); err == nil {
			t.Fatal("corruption accepted", i)
		}
	}
	encode := func(info ArtifactInfo, payload []byte) []byte {
		b, _ := json.Marshal(info)
		out := []byte(artifactMagic)
		out = binary.LittleEndian.AppendUint32(out, uint32(len(b)))
		out = append(out, b...)
		out = append(out, payload...)
		sum := sha256.Sum256(out)
		return append(out, sum[:]...)
	}
	for name, change := range map[string]func(*ArtifactInfo){
		"format":                func(i *ArtifactInfo) { i.Format++ },
		"version":               func(i *ArtifactInfo) { i.Metadata.UpstreamVersion = "go1.27.0" },
		"pin":                   func(i *ArtifactInfo) { i.Metadata.UpstreamCommit = "other" },
		"language":              func(i *ArtifactInfo) { i.Metadata.Language = "go1.28" },
		"target":                func(i *ArtifactInfo) { i.Metadata.Target.GOARCH = "arm64" },
		"stage":                 func(i *ArtifactInfo) { i.Metadata.Stage = "lowered" },
		"experiments":           func(i *ArtifactInfo) { i.Metadata.Experiments = "unknown" },
		"package path identity": func(i *ArtifactInfo) { i.ImportPath = "different" },
		"package name identity": func(i *ArtifactInfo) { i.PackageName = "different" },
		"empty package":         func(i *ArtifactInfo) { i.ImportPath = "" },
		"dependency order": func(i *ArtifactInfo) {
			i.Dependencies = []ArtifactDependency{{Path: "z", Kind: "artifact"}, {Path: "a", Kind: "artifact"}}
		},
		"dependency kind": func(i *ArtifactInfo) { i.Dependencies = []ArtifactDependency{{Path: "dep", Kind: "unknown"}} },
	} {
		t.Run(name, func(t *testing.T) {
			info := p.Artifact.Info()
			change(&info)
			if _, err := DecodeArtifact(encode(info, []byte(p.Artifact.payload))); err == nil {
				t.Fatal("invalid metadata accepted")
			}
		})
	}
	for name, change := range map[string]func([]byte){
		"version":       func(b []byte) { binary.LittleEndian.PutUint32(b, 99) },
		"flags":         func(b []byte) { binary.LittleEndian.PutUint32(b[4:], 2) },
		"allocation":    func(b []byte) { binary.LittleEndian.PutUint32(b[44:], 0xffffffff) },
		"section order": func(b []byte) { binary.LittleEndian.PutUint32(b[8:], 0xffffffff) },
		"extent":        func(b []byte) { binary.LittleEndian.PutUint32(b[48:], 0xffffffff) },
	} {
		t.Run("payload "+name, func(t *testing.T) {
			b := []byte(p.Artifact.payload)
			change(b)
			sum := sha256.Sum256(b[:len(b)-8])
			copy(b[len(b)-8:], sum[:8])
			if _, err := DecodeArtifact(encode(p.Artifact.Info(), b)); err == nil {
				t.Fatal("invalid native envelope accepted")
			}
		})
	}
	if _, err := DecodeArtifact(encode(p.Artifact.Info(), []byte("short"))); err == nil {
		t.Fatal("short payload accepted")
	}
	if _, err := DecodeArtifact(good); err != nil {
		t.Fatal("failure affected valid artifact", err)
	}
}

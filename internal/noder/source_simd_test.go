package noder

import (
	"bytes"
	"fmt"
	"go-backend-ssa/internal/ir"
	"go-backend-ssa/internal/syntax"
	"go-backend-ssa/internal/types2"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Ordinary source stand-ins give this test real compiler archives without
// claiming hardware SIMD execution. Midway recognizes the native package/type
// identities and must reload both the original and injected bridge dependencies.
func TestSourceMidwayRechecksAndReloads(t *testing.T) {
	dir := t.TempDir()
	archives := map[string][]byte{}
	source := map[string]string{"simd": `package simd;type Int8s [16]int8;func VectorBitSize()int{return 128};func Emulated()bool{return false}`, "simd/internal/bridge": `package bridge;type Int8s [16]int8;type Int8x16 [16]int8;type Int8x32 [32]int8;type Int8x64 [64]int8`}
	for i, path := range []string{"simd", "simd/internal/bridge"} {
		file := filepath.Join(dir, fmt.Sprintf("vector%d.go", i))
		archive := file + ".a"
		if e := os.WriteFile(file, []byte(source[path]), 0600); e != nil {
			t.Fatal(e)
		}
		cmd := exec.Command("go", "tool", "compile", "-p", path, "-pack", "-o", archive, file)
		cmd.Env = append(os.Environ(), "GOWORK=off", "GOTOOLCHAIN=go1.27.1", "GOOS=linux", "GOARCH=amd64", "GOAMD64=v1", "GOEXPERIMENT=none,simd")
		if b, e := cmd.CombinedOutput(); e != nil {
			t.Fatalf("%v\n%s", e, b)
		}
		b, e := os.ReadFile(archive)
		if e != nil {
			t.Fatal(e)
		}
		archives[path] = b
	}
	c := bodyContext(t)
	c.Ctxt.TargetOS = "linux"
	c.Types.Experiment.SIMD = true
	c.Types.Experiment.RegabiArgs = true
	c.Types.Experiment.RegabiWrappers = true
	c.Flag.LowerL = 1
	opens := map[string]int{}
	closes := 0
	c.Flag.Cfg.PackageFile = map[string]string{"simd": "simd", "simd/internal/bridge": "simd/internal/bridge"}
	c.BindImports(ObjectConfig{GOOS: "linux", GOARCH: "amd64", GOAMD64: 1, Version: "go1.27.1"}, "linux", func(path string) (DependencyFile, error) {
		opens[path]++
		data, ok := archives[path]
		if !ok {
			return nil, os.ErrNotExist
		}
		return &dependencyMemory{Reader: bytes.NewReader(data), name: path, closes: &closes}, nil
	})
	c.Debug.AstDump = "Count"
	report := new(syntaxOutput)
	c.OpenSyntaxHTML = func(string, int, fs.FileMode) (io.WriteCloser, error) { return report, nil }
	c.SyntaxWorkingDirectory = func() (string, error) { return "/reports", nil }
	f := parseSource(t, c, "input.go", `package p;import "simd";func Identity(v simd.Int8s)simd.Int8s{return v};func Count()int{var v simd.Int8s;return len(v)}`)
	target, e := c.CompileParsedPackage([]*ParsedFile{f})
	if e != nil {
		t.Fatal(e)
	}
	if opens["simd"] != 2 || opens["simd/internal/bridge"] != 1 || closes != 3 {
		t.Fatalf("wrong reloads: %v, closes=%d", opens, closes)
	}
	if len(target.Imports) != 2 || !target.Imports[0].Direct || !target.Imports[1].Direct {
		t.Fatal("direct imports were not rebuilt")
	}
	for _, size := range []int{0, 128, 256, 512} {
		name := fmt.Sprintf("Count@simd%d", size)
		fn := c.Types.LocalPkg.Lookup(name).Def
		if fn == nil || fn.(*ir.Name).Func.Typecheck() != 1 {
			t.Fatalf("missing typed specialization %s", name)
		}
	}
	if !strings.Contains(report.String(), "midway before recheck") || !strings.Contains(report.String(), "midway after recheck") || report.closed != 1 {
		t.Fatal("missing intermediate reports")
	}
	// Definition names need not carry expression types. Evaluated basic
	// expressions must use the fresh checker's predeclared type identities.
	basics := 0
	syntax.Inspect(f.Syntax(), func(n syntax.Node) bool {
		if e, ok := n.(syntax.Expr); ok {
			if b, ok := e.GetTypeInfo().Type.(*types2.Basic); ok {
				basics++
				if b != c.Checker.Typ[b.Kind()] {
					t.Fatal("stale predeclared type after recheck")
				}
			}
		}
		return true
	})
	if basics == 0 {
		t.Fatal("no evaluated expressions checked")
	}
}

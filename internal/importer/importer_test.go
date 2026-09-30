package importer

import (
	"bufio"
	"bytes"
	"fmt"
	"go-backend-ssa/internal/go/exportdata"
	"go-backend-ssa/internal/go/pkgbits"
	"go-backend-ssa/internal/syntax"
	"go-backend-ssa/internal/types2"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// Only the test creates native archive fixtures. Production accepts bytes and
// never locates a toolchain, reads a file, or executes a compiler.
func exportFixtures(t *testing.T, answer int) map[string]string {
	t.Helper()
	dir := t.TempDir()
	out := map[string]string{}
	for _, name := range []string{"a", "b"} {
		source, e := os.ReadFile("testdata/" + name + ".go.txt")
		if e != nil {
			t.Fatal(e)
		}
		source = bytes.ReplaceAll(source, []byte("Answer = 41"), []byte(fmt.Sprintf("Answer = %d", answer)))
		file := filepath.Join(dir, name+".go")
		archive := filepath.Join(dir, name+".a")
		if e = os.WriteFile(file, source, 0600); e != nil {
			t.Fatal(e)
		}
		args := []string{"tool", "compile", "-p", "example/" + name, "-trimpath", dir + "=>/src", "-pack", "-o", archive}
		if name == "b" {
			cfg := filepath.Join(dir, "importcfg")
			if e = os.WriteFile(cfg, []byte("packagefile example/a="+filepath.Join(dir, "a.a")+"\n"), 0600); e != nil {
				t.Fatal(e)
			}
			args = append(args, "-importcfg", cfg)
		}
		args = append(args, file)
		cmd := exec.Command("go", args...)
		cmd.Env = append(os.Environ(), "GOWORK=off", "GOTOOLCHAIN=go1.27.1", "GOOS=linux", "GOARCH=amd64", "GOEXPERIMENT=")
		if b, e := cmd.CombinedOutput(); e != nil {
			t.Fatalf("native export: %v\n%s", e, b)
		}
		raw, e := os.ReadFile(archive)
		if e != nil {
			t.Fatal(e)
		}
		start, end := bytes.Index(raw, []byte("\n$$B\nu")), bytes.Index(raw, []byte("\n$$\n"))
		if start < 0 || end <= start {
			t.Fatal("invalid native export archive")
		}
		payload, err := exportdata.ReadUnified(bufio.NewReader(bytes.NewReader(raw)))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(payload, raw[start+len("\n$$B\nu"):end]) {
			t.Fatal("native archive payload disagrees with independent fixture extraction")
		}
		out[name] = string(payload)
	}
	return out
}
func TestNativeLazyImportsAndSourceChecking(t *testing.T) {
	data := exportFixtures(t, 41)
	next := exportFixtures(t, 72)
	var retained [8]*types2.Named
	var wg sync.WaitGroup
	for i := range retained {
		wg.Go(func() {
			checker := types2.NewContext()
			c := NewContext(checker)
			b := c.ReadExport("example/b", data["b"])
			value := types2.Unalias(b.Scope().Lookup("Value").Type()).(*types2.Named)
			a := value.Obj().Pkg()
			if a.Complete() {
				t.Error("dependency completed without loading its export")
			}
			_ = value.Underlying()
			retained[i] = value
			if c.ReadExport("example/a", data["a"]) != a || !a.Complete() {
				t.Error("transitive package identity changed")
			}
			origin := a.Scope().Lookup("Node").Type().(*types2.Named)
			inst, e := checker.Instantiate(checker, origin, []types2.Type{checker.Typ[types2.Int]}, true)
			if e != nil || !checker.Identical(value, inst) {
				t.Error("instantiation identity", e)
			}
			if origin.NumMethods() != 2 {
				t.Error("missing generic method")
			}
			_ = a.Scope().Lookup("A").Type().Underlying()
			_ = a.Scope().Lookup("B").Type().Underlying()
			source := `package use;import "example/a";import "example/b";var X a.Node[int]=b.Value;var Y=a.Global.Map(func(x int)string{return "x"});func F(x a.Scalar)int{return x.Value()};var Z=a.Generic(int64(3))`
			f, e := syntax.Parse(syntax.NewFileBase("use.go"), strings.NewReader(source), nil, nil, syntax.CheckBranches)
			if e != nil {
				t.Error(e)
				return
			}
			cfg := types2.Config{Context: checker, GoVersion: "go1.27", Importer: c, Sizes: checker.SizesFor("gc", "amd64")}
			pkg, e := cfg.Check("example/use", []*syntax.File{f}, nil)
			if e != nil {
				t.Error(e)
				return
			}
			if pkg.Scope().Lookup("Y").Type().String() != "example/a.Node[string]" {
				t.Error("generic method check failed")
			}
			other := NewContext(types2.NewContext())
			newA := other.ReadExport("example/a", next["a"])
			if newA.Scope().Lookup("Answer").(*types2.Const).Val().ExactString() != "72" || a.Scope().Lookup("Answer").(*types2.Const).Val().ExactString() != "41" {
				t.Error("export data leaked between instances")
			}
			if b.Scope().Lookup("Value").Type() != value && types2.Unalias(b.Scope().Lookup("Value").Type()) != value {
				t.Error("retained lazy object changed")
			}
		})
	}
	wg.Wait()
	for i, n := range retained {
		if n == nil {
			continue
		}
		if n.Obj().Pkg().Scope().Lookup("Answer").(*types2.Const).Val().ExactString() != "41" {
			t.Fatal("retained graph invalid")
		}
		for j := 0; j < i; j++ {
			if n == retained[j] {
				t.Fatal("shared type across instances")
			}
		}
	}
}
func TestImportBoundaries(t *testing.T) {
	data := exportFixtures(t, 41)
	checker := types2.NewContext()
	c := NewContext(checker)
	mustPanic := func(f func()) {
		t.Helper()
		defer func() {
			if recover() == nil {
				t.Error("invalid import accepted")
			}
		}()
		f()
	}
	foreign := types2.NewContext()
	packages := map[string]*types2.Package{"foreign": foreign.NewPackage("foreign", "foreign")}
	decoder := pkgbits.NewPkgDecoder("example/a", data["a"])
	mustPanic(func() { c.ReadPackage(checker, packages, decoder) })
	if len(packages) != 1 {
		t.Fatal("foreign map mutated")
	}
	mustPanic(func() { c.ReadPackage(foreign, map[string]*types2.Package{}, decoder) })
	mustPanic(func() { c.ReadPackage(checker, nil, decoder) })
	if _, e := c.Import("missing"); e == nil {
		t.Fatal("missing dependency accepted")
	}
	a := c.ReadExport("example/a", data["a"])
	if p, e := c.Import("example/a"); e != nil || p != a {
		t.Fatal("import cache", e)
	}
	if p, e := c.Import("unsafe"); e != nil || p != checker.Unsafe {
		t.Fatal("unsafe identity")
	}
	mustPanic(func() { c.ReadExport("example/a", data["a"]) })
	mustPanic(func() { NewContext(types2.NewContext()).ReadExport("example/bad", "invalid") })
	fresh := NewContext(types2.NewContext())
	if fresh.ReadExport("example/a", data["a"]).Scope().Len() == 0 {
		t.Fatal("failure affected independent import")
	}
}

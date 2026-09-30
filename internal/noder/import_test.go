package noder

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"go-backend-ssa/internal/go/exportdata"
	"go-backend-ssa/internal/ir"
	"go-backend-ssa/internal/syntax"
	"go-backend-ssa/internal/types2"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
)

type dependencyMemory struct {
	*bytes.Reader
	name   string
	closes *int
	data   []byte
	wipe   bool
}

func (f *dependencyMemory) Name() string { return f.name }
func (f *dependencyMemory) Close() error {
	*f.closes++
	if f.wipe {
		clear(f.data)
	}
	return nil
}
func dependencyArchives(t *testing.T) map[string][]byte {
	t.Helper()
	dir := t.TempDir()
	out := map[string][]byte{}
	for _, name := range []string{"a", "b"} {
		source := `package a;type Box[T any]struct{Value T};func(b Box[T])Get()T{return b.Value};func Add(x int)int{return x+3};const Answer=41;var Global=Box[int]{Answer};func init(){Global.Value++}`
		if name == "b" {
			source = `package b;import "example/a";var Value a.Box[int];func Sum(x int)int{return a.Add(x)+2}`
		}
		file := filepath.Join(dir, name+".go")
		archive := filepath.Join(dir, name+".a")
		if e := os.WriteFile(file, []byte(source), 0600); e != nil {
			t.Fatal(e)
		}
		args := []string{"tool", "compile", "-p", "example/" + name, "-trimpath", dir + "=>/src", "-pack", "-o", archive}
		if name == "b" {
			cfg := filepath.Join(dir, "importcfg")
			if e := os.WriteFile(cfg, []byte("packagefile example/a="+filepath.Join(dir, "a.a")+"\n"), 0600); e != nil {
				t.Fatal(e)
			}
			args = append(args, "-importcfg", cfg)
		}
		args = append(args, file)
		cmd := exec.Command("go", args...)
		cmd.Env = append(os.Environ(), "GOWORK=off", "GOTOOLCHAIN=go1.27.1", "GOOS=linux", "GOARCH=amd64", "GOAMD64=v1", "GOEXPERIMENT=none")
		if b, e := cmd.CombinedOutput(); e != nil {
			t.Fatalf("%v\n%s", e, b)
		}
		b, e := os.ReadFile(archive)
		if e != nil {
			t.Fatal(e)
		}
		out[name] = b
	}
	return out
}
func dependencyContext(t *testing.T, data map[string][]byte, wipe bool) (*Context, *[]string, *int) {
	t.Helper()
	c := bodyContext(t)
	c.Ctxt.TargetOS = "linux"
	c.Types.Experiment.RegabiArgs = true
	c.Types.Experiment.RegabiWrappers = true
	c.Flag.LowerL = 1
	var opens []string
	closes := 0
	c.Flag.Cfg.PackageFile = map[string]string{"example/a": "a", "example/b": "b"}
	c.BindImports(ObjectConfig{GOOS: "linux", GOARCH: "amd64", GOAMD64: 1, Version: "go1.27.1"}, "linux", func(path string) (DependencyFile, error) {
		opens = append(opens, path)
		data, ok := data[path]
		if !ok {
			return nil, os.ErrNotExist
		}
		data = append([]byte(nil), data...)
		return &dependencyMemory{Reader: bytes.NewReader(data), name: path, closes: &closes, data: data, wipe: wipe}, nil
	})
	return c, &opens, &closes
}
func TestNativeDependencyImportsReachIR(t *testing.T) {
	data := dependencyArchives(t)
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			c, opens, closes := dependencyContext(t, data, true)
			b, e := c.Import("example/b")
			if e != nil {
				t.Error(e)
				return
			}
			value := types2.Unalias(b.Scope().Lookup("Value").Type()).(*types2.Named)
			a := value.Obj().Pkg()
			if a.Complete() {
				t.Error("lazy dependency completed early")
			}
			a2, e := c.Import("example/a")
			if e != nil || a != a2 || !a.Complete() {
				t.Error("dependency identity", a2, e)
			}
			if a.Scope().Lookup("Answer").(*types2.Const).Val().ExactString() != "41" {
				t.Error("retained archive payload corrupted")
			}
			if again, e := c.Import("example/b"); again != b || e != nil {
				t.Error("cache identity", e)
			}
			if _, e = c.Import("unsafe"); e != nil {
				t.Error(e)
			}
			if !reflect.DeepEqual(*opens, []string{"b", "a"}) || *closes != 2 {
				t.Error(*opens, *closes)
			}
			if len(c.Typecheck.Target.Imports) != 3 || !c.Types.NewPkg("example/a", "").Direct || !c.Types.NewPkg("example/b", "").Direct {
				t.Error("native imports")
			}
			if len(c.Ctxt.Imports) != 2 {
				t.Error("missing fingerprints", c.Ctxt.Imports)
			}
			for i, name := range []string{"b", "a"} {
				raw, e := exportdata.ReadUnified(bufio.NewReader(bytes.NewReader(data[name])))
				if e != nil {
					t.Error(e)
					return
				}
				got := c.Ctxt.Imports[i]
				if got.Pkg != "example/"+name || !bytes.Equal(got.Fingerprint[:], raw[len(raw)-8:]) {
					t.Error("fingerprint", got)
				}
			}
			source := `package p;import "example/a";import "example/b";func F(x int)int{return a.Add(x)+b.Sum(x)+a.Global.Get()}`
			f, e := c.ParseFile("p.go", strings.NewReader(source))
			if e != nil {
				t.Error(e)
				return
			}
			info := &types2.Info{StoreTypesInSyntax: true, Defs: map[*syntax.Name]types2.Object{}, Uses: map[*syntax.Name]types2.Object{}, Selections: map[*syntax.SelectorExpr]*types2.Selection{}, Implicits: map[syntax.Node]types2.Object{}, Scopes: map[syntax.Node]*types2.Scope{}, Instances: map[*syntax.Name]types2.Instance{}, FileVersions: map[*syntax.PosBase]string{}}
			cfg := types2.Config{Context: c.Checker, GoVersion: "go1.27", Sizes: c.Checker.SizesFor("gc", "amd64"), Importer: c}
			pkg, e := cfg.Check("example/p", []*syntax.File{f.Syntax()}, info)
			if e != nil {
				t.Error(e)
				return
			}
			closures, e := c.RewriteRangeFuncs(pkg, info, []*syntax.File{f.Syntax()})
			if e != nil {
				t.Error(e)
				return
			}
			stub, e := c.WriteParsedPackage(pkg, info, []*ParsedFile{f}, closures)
			if e != nil {
				t.Error(e)
				return
			}
			c.ReadLocalPackage(stub)
			fn := c.Types.LocalPkg.Lookup("F").Def.(*ir.Name).Func
			if len(fn.Body) == 0 || fn.Typecheck() == 0 {
				t.Error("local body not read")
			}
			c.Interleaved.DevirtualizeAndInlinePackage(c.Typecheck.Target, nil)
			if c.Diagnostics.HasErrors() {
				t.Error(c.Diagnostics.Diagnostics())
			}
			if *closes != 2 {
				t.Error("lazy body reopened archives")
			}
		})
	}
	wg.Wait()
}
func TestDependencyFailuresInvalidateContext(t *testing.T) {
	data := dependencyArchives(t)
	cases := map[string][]byte{"wrong header": bytes.Replace(data["a"], []byte("linux"), []byte("xxxxx"), 1), "truncated": data["a"][:30], "missing marker": bytes.Replace(data["a"], []byte("\n$$\n"), []byte("\nXX\n"), 1)}
	for name, b := range cases {
		t.Run(name, func(t *testing.T) {
			c, opens, closes := dependencyContext(t, map[string][]byte{"a": b}, false)
			if _, e := c.Import("example/a"); e == nil {
				t.Fatal("invalid archive accepted")
			}
			if !c.importFailed || *closes != 1 {
				t.Fatal("failure state", c.importFailed, *closes)
			}
			if _, e := c.Import("example/a"); e == nil || len(*opens) != 1 {
				t.Fatal("failed native import was reused")
			}
		})
	}
	c, opens, _ := dependencyContext(t, data, false)
	c.Checker = types2.NewContext()
	if _, e := c.Import("example/a"); e == nil || len(*opens) != 0 {
		t.Fatal("foreign checker accepted")
	}
	c, _, _ = dependencyContext(t, data, false)
	marker := &struct{}{}
	c.dependencyOpen = func(string) (DependencyFile, error) { panic(marker) }
	func() {
		defer func() {
			if recover() != marker {
				t.Error("provider panic replaced")
			}
		}()
		c.Import("example/a")
	}()
	if !c.importFailed {
		t.Fatal("panic did not invalidate context")
	}
	c, _, _ = dependencyContext(t, data, false)
	c.dependencyOpen = func(string) (DependencyFile, error) { return nil, nil }
	if _, e := c.Import("example/a"); e == nil {
		t.Fatal("nil stream accepted")
	}
	c, _, _ = dependencyContext(t, data, false)
	failure := errors.New("provider")
	c.dependencyOpen = func(string) (DependencyFile, error) { return nil, failure }
	if _, e := c.Import("example/a"); e != failure {
		t.Fatal("provider error identity", e)
	}
}
func TestNativeDependencyPathAndSearch(t *testing.T) {
	for _, host := range []string{"linux", "windows"} {
		c, _, _ := dependencyContext(t, nil, false)
		c.hostOS = host
		for _, row := range []struct {
			path  string
			local bool
		}{{"x", false}, {"./x", true}, {"../x", true}, {"/x", true}, {"C:/x", host == "windows"}} {
			if c.islocalname(row.path) != row.local {
				t.Error(host, row)
			}
		}
		for _, path := range []string{"main", "example/p", "/x"} {
			if _, e := c.resolveImportPath(path); e == nil {
				t.Error("invalid path", path)
			}
		}
		c.Flag.Cfg.ImportMap = map[string]string{"alias": "example/a"}
		if p, e := c.resolveImportPath("alias"); p != "example/a" || e != nil {
			t.Error(p, e)
		}
		c.Flag.D = "prefix"
		if p, e := c.resolveImportPath("./a"); p != "prefix/a" || e != nil {
			t.Error(p, e)
		}
		for _, path := range []string{"", "bad\x00path", "go", "a b", "bad\\path", "bad\xff", "x:y"} {
			if e := c.checkImportPath(path, false); e == nil {
				t.Error("invalid import", path)
			}
		}
	}
	for _, mode := range []string{"dirs", "suffix", "race", "msan", "asan", "local", "mapped", "canonical", "nolocal"} {
		t.Run(mode, func(t *testing.T) {
			c, _, _ := dependencyContext(t, nil, false)
			c.Flag.Cfg.PackageFile = nil
			c.Flag.Cfg.ImportDirs = []string{"first", "second"}
			c.GOROOT = "sdk"
			path := "example/a"
			var calls []string
			c.dependencyOpen = func(p string) (DependencyFile, error) { calls = append(calls, p); return nil, os.ErrNotExist }
			suffix := ""
			switch mode {
			case "suffix":
				c.Flag.InstallSuffix = "custom"
				suffix = "_custom"
			case "race":
				c.Flag.Race = true
				suffix = "_race"
			case "msan":
				c.Flag.MSan = true
				suffix = "_msan"
			case "asan":
				c.Flag.ASan = true
				suffix = "_asan"
			case "local":
				path = "./a"
			case "mapped":
				c.Flag.Cfg.PackageFile = map[string]string{path: "blob"}
			case "canonical":
				path = "example/../a"
			case "nolocal":
				path = "./a"
				c.Flag.NoLocalImports = true
			}
			if _, e := c.openPackage(path); e == nil {
				t.Fatal("missing input accepted")
			}
			want := []string{"first/example/a.a", "first/example/a.o", "second/example/a.a", "second/example/a.o", fmt.Sprintf("sdk/pkg/linux_amd64%s/example/a.a", suffix), fmt.Sprintf("sdk/pkg/linux_amd64%s/example/a.o", suffix)}
			switch mode {
			case "local":
				want = []string{"./a.a", "./a.o"}
			case "mapped":
				want = []string{"blob"}
			case "canonical", "nolocal":
				want = nil
			}
			if !reflect.DeepEqual(calls, want) {
				t.Fatal(calls, want)
			}
		})
	}
}

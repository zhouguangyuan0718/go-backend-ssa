package noder

import (
	"bytes"
	"fmt"
	"go-backend-ssa/internal/cmd/pgo"
	"go-backend-ssa/internal/go/pkgbits"
	"go-backend-ssa/internal/ir"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// The subprocess is test-only. Production registration accepts the caller's
// native export data and never invokes a compiler or reads an archive/file.
func compilerExport(t *testing.T, pkgpath, source string) string {
	t.Helper()
	dir := t.TempDir()
	file := filepath.Join(dir, "dep.go")
	archive := filepath.Join(dir, "dep.a")
	if err := os.WriteFile(file, []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	run := func(args ...string) []byte {
		t.Helper()
		cmd := exec.Command("go", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GOWORK=off", "GOTOOLCHAIN=go1.27.1", "GOOS=linux", "GOARCH=amd64", "GOEXPERIMENT=")
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("%v: %v\n%s", args, err, out)
		}
		return out
	}
	run("tool", "compile", "-p", pkgpath, "-pack", "-o", archive, file)
	data := run("tool", "pack", "p", archive, "__.PKGDEF")
	start, end := bytes.Index(data, []byte("\n$$B\nu")), bytes.LastIndex(data, []byte("\n$$\n"))
	if start < 0 || end <= start {
		t.Fatalf("not native unified export: %.100s", data)
	}
	return string(data[start+len("\n$$B\nu") : end])
}

func localStub(t *testing.T, c *Context, source string) string {
	t.Helper()
	pkg, info, files := checked(t, c.Checker, source)
	closures, err := c.RewriteRangeFuncs(pkg, info, files)
	if err != nil {
		t.Fatal(err)
	}
	data, err := c.WritePackage(pkg, info, files, closures)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestNativePackageRegistrationAndLookup(t *testing.T) {
	data := compilerExport(t, "example/dep", `package dep
 type Number int
 func(n Number)M(x int)int{return int(n)+x}
 func(n *Number)P(x int)int{return int(*n)+x}
 type Alias=Number
 type I interface{M(int)int}
 type Box[T any]struct{V T}
 func(b Box[T])Get()T{return b.V}
 func Generic[T any](x T)T{return x}
 func Exported(x int)int{return x+1}
 var State=Exported(3)
 func init(){State++}
 `)
	const count = 8
	retained := make([]*ir.Func, count)
	var wg sync.WaitGroup
	for i := range count {
		wg.Go(func() {
			c := bodyContext(t)
			dep := c.Types.NewPkg("example/dep", "")
			c.RegisterPackage(dep, data, false)
			if dep.Name != "dep" || dep.Lookup(".inittask").Def == nil || len(c.importBodyReader) == 0 {
				t.Error("import roots not registered")
				return
			}
			for _, name := range []string{"example/dep.Exported", "example/dep.Number.M", "example/dep.(*Number).P"} {
				fn, err := c.LookupFunc(name)
				if err != nil {
					t.Error(name, err)
					continue
				}
				again, err := c.LookupFunc(name)
				if err != nil || again != fn {
					t.Error("lookup identity", err)
				}
				if !c.Typecheck.IR.OwnsNode(fn) || fn.Inl == nil || !c.Typecheck.HaveInlineBody(fn) {
					t.Error("imported native inline body missing", name)
				}
				if name == "example/dep.Exported" {
					retained[i] = fn
				}
			}
			for _, name := range []string{"example/dep.State", "example/dep.Alias.M", "example/dep.I.M", "example/dep.Missing", "example/dep.Generic", "unknown/pkg.F", "badname"} {
				if fn, err := c.LookupFunc(name); err == nil || fn != nil {
					t.Error("invalid lookup accepted", name, fn)
				}
			}
			c.PostLookupCleanup()
			if len(c.Typecheck.Target.Funcs) != 0 {
				t.Error("imported ordinary functions scheduled for local compilation")
			}
			// PGO looks up an otherwise unreferenced imported function through the real
			// production noder hook, then runs the real pending-body cleanup hook.
			source := fmt.Sprintf("package p;func Local(x int)int{return x+%d}", i)
			target := c.ReadLocalPackage(localStub(t, c, source))
			if target != c.Typecheck.Target || len(target.Funcs) != 1 {
				t.Error("local package reader target")
				return
			}
			profileInput, err := pgo.FromSerialized(strings.NewReader("GO PREPROFILE V1\nexample/p.Local\nexample/dep.Exported\n0 100\n"))
			if err != nil {
				t.Error(err)
				return
			}
			profile := c.Inline.PGO.NewProfile(profileInput)
			if profile.WeightedCG.IRNodes["example/dep.Exported"].AST != retained[i] {
				t.Error("PGO did not use native lookup")
			}
			if _, err := c.LookupFunc("example/p.Local"); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	for i, fn := range retained {
		if fn == nil {
			t.Fatal("missing retained imported function")
		}
		if fn.Sym().Pkg.Path != "example/dep" {
			t.Fatal("retained symbol changed")
		}
		for j := 0; j < i; j++ {
			if fn == retained[j] || fn.Type() == retained[j].Type() {
				t.Fatal("imported objects shared")
			}
		}
	}
}

func TestReadLocalPackageAndRegistrationFailures(t *testing.T) {
	c := bodyContext(t)
	target := c.ReadLocalPackage(localStub(t, c, `package p;type Box[T any]struct{V T};func Id[T any](x T)T{return x};var X=Id(Box[int]{V:3});func F()int{return X.V};func init(){X.V++}`))
	if target != c.Typecheck.Target || len(target.Funcs) < 2 || len(target.Inits) == 0 || len(c.objReader) == 0 {
		t.Fatal("local package reader did not produce IR")
	}
	fn, err := c.LookupFunc("example/p.F")
	if err != nil || len(fn.Body) == 0 || fn.Typecheck() != 1 {
		t.Fatal("local lookup", fn, err)
	}
	foreign := bodyContext(t)
	for _, run := range []func(){
		func() { foreign.RegisterPackage(c.Types.LocalPkg, "", false) },
		func() { c.RegisterPackage(c.Types.LocalPkg, "", true) },
		func() { foreign.RegisterPackage(foreign.Types.NewPkg("wrong", ""), "", true) },
	} {
		func() {
			defer func() {
				if recover() == nil {
					t.Error("invalid package registration accepted")
				}
			}()
			run()
		}()
	}
	if _, err := foreign.LookupFunc("example/p.F"); err == nil {
		t.Fatal("failed registration leaked symbols")
	}
	if c.Diagnostics.HasErrors() {
		t.Fatal("another compilation changed diagnostics")
	}
}

func TestNativePackageMismatchAbortsOnlyInstance(t *testing.T) {
	c := bodyContext(t)
	data := localStub(t, c, "package p;func F(){}")
	pr := c.newPkgReader(pkgbits.NewPkgDecoder("example/p", data))
	func() {
		defer func() {
			if v := recover(); v == nil || !strings.Contains(fmt.Sprint(v), "mismatched import path") {
				t.Errorf("missing native diagnostic abort: %v", v)
			}
		}()
		c.readPackage(pr, c.Types.NewPkg("wrong/path", ""), true)
	}()
	if !c.Diagnostics.HasErrors() {
		t.Fatal("native import diagnostic missing")
	}
	other := bodyContext(t)
	target := other.ReadLocalPackage(localStub(t, other, "package p;func F(){}"))
	if other.Diagnostics.HasErrors() || len(target.Funcs) != 1 {
		t.Fatal("aborted compiler contaminated next compilation")
	}
}

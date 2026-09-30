package noder

import (
	"fmt"
	"go-backend-ssa/internal/ir"
	"go-backend-ssa/internal/syntax"
	"go-backend-ssa/internal/types2"
	"strings"
	"sync"
	"testing"
)

type directiveImporter struct{ c *types2.Context }

func (i directiveImporter) Import(path string) (*types2.Package, error) {
	if path == "unsafe" {
		return i.c.Unsafe, nil
	}
	return nil, fmt.Errorf("unexpected import %s", path)
}
func checkParsed(t *testing.T, c *Context, files ...*ParsedFile) (*types2.Package, *types2.Info) {
	t.Helper()
	var trees []*syntax.File
	for _, f := range files {
		trees = append(trees, f.Syntax())
	}
	info := &types2.Info{StoreTypesInSyntax: true, Defs: map[*syntax.Name]types2.Object{}, Uses: map[*syntax.Name]types2.Object{}, Selections: map[*syntax.SelectorExpr]*types2.Selection{}, Implicits: map[syntax.Node]types2.Object{}, Scopes: map[syntax.Node]*types2.Scope{}, Instances: map[*syntax.Name]types2.Instance{}, FileVersions: map[*syntax.PosBase]string{}}
	cfg := types2.Config{Context: c.Checker, GoVersion: "go1.27", Sizes: c.Checker.SizesFor("gc", c.GOARCH), Importer: directiveImporter{c.Checker}}
	pkg, e := cfg.Check(c.Ctxt.Pkgpath, trees, info)
	if e != nil {
		t.Fatal(e)
	}
	return pkg, info
}
func TestParsedDirectivesReachIR(t *testing.T) {
	var wg sync.WaitGroup
	for i := range 8 {
		wg.Go(func() {
			c := bodyContext(t)
			c.GOOS = "linux"
			source := fmt.Sprintf("package p\nimport _ \"unsafe\"\n//go:linkname F target%d.F\n//go:noinline\n//go:nosplit\nfunc F(x int) int {return x+%d}\n//go:linkname V\nvar V int\n//go:cgo_import_dynamic puts puts \"libc.so\"\n", i, i)
			f, e := c.ParseFile("p.go", strings.NewReader(source))
			if e != nil {
				t.Error(e)
				return
			}
			pkg, info := checkParsed(t, c, f)
			data, e := c.WriteParsedPackage(pkg, info, []*ParsedFile{f}, nil)
			if e != nil {
				t.Error(e)
				return
			}
			c.ReadLocalPackage(data)
			fn := c.Types.LocalPkg.Lookup("F").Def.(*ir.Name).Func
			if fn.Pragma&(ir.Noinline|ir.Nosplit|ir.NoCheckPtr) != (ir.Noinline|ir.Nosplit|ir.NoCheckPtr) || fn.Sym().Linkname != fmt.Sprintf("target%d.F", i) {
				t.Error("function directives lost", fn.Pragma, fn.Sym().Linkname)
			}
			if c.Types.LocalPkg.Lookup("V").Linkname != "example/p.V" {
				t.Error("implicit linkname lost")
			}
			if fmt.Sprint(c.Typecheck.Target.CgoPragmas) != "[[cgo_import_dynamic puts puts libc.so]]" {
				t.Error("cgo directive lost", c.Typecheck.Target.CgoPragmas)
			}
			if pkg.Scope().Lookup("F") == nil || f.Syntax().PkgName.Value != "p" {
				t.Error("retained source graph cleared")
			}
		})
	}
	wg.Wait()
}
func TestParsedDirectiveFailuresAndOwnership(t *testing.T) {
	c := bodyContext(t)
	c.GOOS = "linux"
	for _, source := range []string{"package p\n//go:systemstack\nfunc F(){}", "package p\n//go:embed \"unterminated\nvar X string", "package p\nfunc F(){break}"} {
		f, e := c.ParseFile("bad.go", strings.NewReader(source))
		if f != nil || e == nil {
			t.Fatalf("invalid source accepted: %q", source)
		}
		if _, ok := e.(ParseErrors); !ok {
			t.Fatalf("errors not preserved: %T", e)
		}
	}
	f, e := c.ParseFile("ok.go", strings.NewReader("package p\n//go:noinline\nfunc F(){}"))
	if e != nil {
		t.Fatal(e)
	}
	pkg, info := checkParsed(t, c, f)
	other := bodyContext(t)
	other.GOOS = "linux"
	if _, e = other.WriteParsedPackage(pkg, info, []*ParsedFile{f}, nil); e == nil {
		t.Fatal("foreign parsed file accepted")
	}
	if _, e = c.WriteParsedPackage(pkg, info, []*ParsedFile{nil}, nil); e == nil {
		t.Fatal("nil parsed file accepted")
	}
	if _, e = c.WriteParsedPackage(pkg, info, []*ParsedFile{f}, nil); e != nil {
		t.Fatal(e)
	}
	if _, e = c.ParseFile("ok.go", nil); e == nil {
		t.Fatal("nil input accepted")
	}
}

type panicDirectiveReader struct{ value any }

func (r panicDirectiveReader) Read([]byte) (int, error) { panic(r.value) }
func TestDirectiveParserPanicAndConcurrentFiles(t *testing.T) {
	c := bodyContext(t)
	c.GOOS = "linux"
	sentinel := new(int)
	func() {
		defer func() {
			if recover() != sentinel {
				t.Error("reader panic changed")
			}
		}()
		c.ParseFile("panic.go", panicDirectiveReader{sentinel})
	}()
	const count = 32
	files := make([]*ParsedFile, count)
	var wg sync.WaitGroup
	for i := range count {
		wg.Go(func() {
			f, e := c.ParseFile(fmt.Sprintf("f%d.go", i), strings.NewReader(fmt.Sprintf("package p\n//go:noinline\nfunc F%d(){}", i)))
			if e != nil {
				t.Error(e)
				return
			}
			files[i] = f
		})
	}
	wg.Wait()
	pkg, info := checkParsed(t, c, files...)
	if _, e := c.WriteParsedPackage(pkg, info, files, nil); e != nil {
		t.Fatal(e)
	}
}

func TestParsedWriterRejectsInvalidLinknames(t *testing.T) {
	for _, test := range []struct{ source, want string }{
		{"package p\n//go:linkname F other.F\nfunc F(){}", "only allowed in Go files that import"},
		{"package p\nimport _ \"unsafe\"\n//go:linkname F other.F\n//go:linkname F another.F\nfunc F(){}", "duplicate //go:linkname"},
		{"package p\nimport _ \"unsafe\"\n//go:linkname F other.F[int]\nfunc F(){}", "reference of an instantiation is not allowed"},
		{"package p\nimport _ \"unsafe\"\n//go:linkname Absent other.F\nfunc F(){}", "must refer to declared function or variable"},
	} {
		t.Run(test.want, func(t *testing.T) {
			c := bodyContext(t)
			c.GOOS = "linux"
			f, e := c.ParseFile("bad.go", strings.NewReader(test.source))
			if e != nil {
				t.Fatal(e)
			}
			pkg, info := checkParsed(t, c, f)
			data, e := c.WriteParsedPackage(pkg, info, []*ParsedFile{f}, nil)
			if data != "" || e == nil || !strings.Contains(e.Error(), test.want) {
				t.Fatalf("writer accepted invalid directive: %q %v", data, e)
			}
		})
	}
}

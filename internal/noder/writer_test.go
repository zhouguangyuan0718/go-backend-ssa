package noder

import (
	"go-backend-ssa/internal/cmd/obj"
	"go-backend-ssa/internal/cmd/sys"
	"go-backend-ssa/internal/go/pkgbits"
	"go-backend-ssa/internal/syntax"
	"go-backend-ssa/internal/types"
	"go-backend-ssa/internal/types2"
	"strings"
	"sync"
	"testing"
)

func checked(t *testing.T, c *types2.Context, source string) (*types2.Package, *types2.Info, []*syntax.File) {
	t.Helper()
	f, err := syntax.Parse(syntax.NewFileBase("input.go"), strings.NewReader(source), nil, nil, syntax.CheckBranches)
	if err != nil {
		t.Fatal(err)
	}
	info := &types2.Info{StoreTypesInSyntax: true, Defs: map[*syntax.Name]types2.Object{}, Uses: map[*syntax.Name]types2.Object{}, Selections: map[*syntax.SelectorExpr]*types2.Selection{}, Implicits: map[syntax.Node]types2.Object{}, Scopes: map[syntax.Node]*types2.Scope{}, Instances: map[*syntax.Name]types2.Instance{}, FileVersions: map[*syntax.PosBase]string{}}
	conf := types2.Config{Context: c, GoVersion: "go1.27", Sizes: c.SizesFor("gc", "amd64")}
	pkg, err := conf.Check("example/p", []*syntax.File{f}, info)
	if err != nil {
		t.Fatal(err)
	}
	return pkg, info, []*syntax.File{f}
}
func writerContext(t *testing.T) *Context {
	t.Helper()
	link := &obj.Link{Pathname: "/src", Arch: &obj.LinkArch{Arch: &sys.Arch{Name: "amd64", PtrSize: 8, RegSize: 8}}}
	c, err := NewContext(types2.NewContext(), types.NewUniverse(8, 8, 1<<50), link)
	if err != nil {
		t.Fatal(err)
	}
	return c
}
func TestWriteCheckedPackage(t *testing.T) {
	for _, source := range []string{
		`package p;func Add(x,y int)int {for i:=0;i<y;i++{if i%2==0{x+=i}};return x}`,
		`package p;type Box[T any]struct{V T};func Id[T any](x T)T{return x};var X=Id(Box[int]{V:3})`,
		`package p;func F(x int)func()int{return func()int{x++;return x}}`,
		`package p;type T int;func(t T)M()int{return int(t)};var I interface{M()int}=T(1)`,
		`package p;func F(x any)(v int){switch x:=x.(type){case int:return x;case string:return len(x)};return 0}`,
	} {
		t.Run(source, func(t *testing.T) {
			c := writerContext(t)
			pkg, info, files := checked(t, c.Checker, source)
			data, err := c.WritePackage(pkg, info, files, nil)
			if err != nil {
				t.Fatal(err)
			}
			pr := pkgbits.NewPkgDecoder("example/p", data)
			if pr.NumElems(pkgbits.SectionMeta) != 2 || pr.NumElems(pkgbits.SectionBody) == 0 {
				t.Fatal("missing roots or bodies")
			}
			if pkg.Scope().Len() == 0 || pkg.Context() != c.Checker {
				t.Fatal("writer cleared checked graph")
			}
			again, err := c.WritePackage(pkg, info, files, nil)
			if err != nil || again != data {
				t.Fatalf("non-repeatable write: %v", err)
			}
		})
	}
}
func TestWriteBoundaries(t *testing.T) {
	c := writerContext(t)
	pkg, info, files := checked(t, c.Checker, `package p;func Each(yield func(int)bool){yield(1)};func F(){for x:=range Each{_=x}}`)
	if _, err := c.WritePackage(pkg, info, files, nil); err == nil || !strings.Contains(err.Error(), "rewriting") {
		t.Fatalf("unlowered rangefunc: %v", err)
	}
	foreign := writerContext(t)
	if _, err := foreign.WritePackage(pkg, info, files, nil); err == nil {
		t.Fatal("foreign checker graph accepted")
	}
}

func TestConcurrentWritersAndRetainedTypes(t *testing.T) {
	var wg sync.WaitGroup
	for range 12 {
		wg.Go(func() {
			c := writerContext(t)
			pkg, info, files := checked(t, c.Checker, `package p;type Box[T any]struct{V T};var X Box[int]`)
			old := pkg.Scope().Lookup("X").Type()
			data, err := c.WritePackage(pkg, info, files, nil)
			if err != nil {
				t.Error(err)
				return
			}
			next, ni, nf := checked(t, c.Checker, `package p;type Box struct{Value string};var X Box`)
			if _, err := c.WritePackage(next, ni, nf, nil); err != nil {
				t.Error(err)
			}
			if pkg.Scope().Lookup("X").Type() != old || next.Scope().Lookup("X").Type() == old || len(data) == 0 {
				t.Error("old graph invalidated")
			}
		})
	}
	wg.Wait()
}
func TestWriterDiagnosticRecovery(t *testing.T) {
	c := writerContext(t)
	bad, bi, bf := checked(t, c.Checker, `package p;func F()`)
	if data, err := c.WritePackage(bad, bi, bf, nil); err == nil || data != "" || !strings.Contains(err.Error(), "missing function body") {
		t.Fatalf("invalid complete package: %q %v", data, err)
	}
	diagnostics := c.Diagnostics.Diagnostics()
	good, gi, gf := checked(t, c.Checker, `package p;func F(){}`)
	if data, err := c.WritePackage(good, gi, gf, nil); err != nil || data == "" || c.Diagnostics.HasErrors() {
		t.Fatalf("recovery: %v", err)
	}
	if len(diagnostics) != 1 || bad.Scope().Lookup("F") == nil {
		t.Fatal("old diagnostic/graph cleared")
	}
}

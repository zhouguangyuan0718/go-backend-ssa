package types2_test

import (
	"bytes"
	"fmt"
	"strings"
	"sync"
	"testing"

	"go-backend-ssa/gcbridge"
	"go-backend-ssa/syntax"
	"go-backend-ssa/types2"
)

type importerFunc func(string) (*types2.Package, error)

func (f importerFunc) Import(path string) (*types2.Package, error) { return f(path) }

func check(c *types2.Context, path, source, arch, version string, importer types2.Importer, trace bool) (*types2.Package, *types2.Info, []error, error) {
	f, err := syntax.Parse(syntax.NewFileBase("input.go"), strings.NewReader(source), nil, nil, syntax.CheckBranches)
	if err != nil {
		return nil, nil, nil, err
	}
	info := &types2.Info{Types: map[syntax.Expr]types2.TypeAndValue{}, Defs: map[*syntax.Name]types2.Object{}, Uses: map[*syntax.Name]types2.Object{}, Instances: map[*syntax.Name]types2.Instance{}, StoreTypesInSyntax: true}
	var diagnostics []error
	conf := types2.Config{Context: c, GoVersion: version, Sizes: c.SizesFor("gc", arch), Importer: importer, Trace: trace, Error: func(err error) { diagnostics = append(diagnostics, err) }}
	pkg, err := conf.Check(path, []*syntax.File{f}, info)
	return pkg, info, diagnostics, err
}

func TestSourceTypeChecking(t *testing.T) {
	for _, source := range []string{
		`package p;func Add(x,y int)int{if x<0{return -x};for i:=0;i<y;i++{x+=i};return x+y}`,
		`package p;type Box[T any]struct{V T};func Id[T any](x T)T{return x};var X=Id(Box[int]{V:1})`,
		`package p;type N int;func(n N)M()int{return int(n)};var X interface{M()int}=N(3)`,
		`package p;func Each(yield func(int)bool){for i:=0;i<3;i++{if !yield(i){return}}};func F()int{sum:=0;for x:=range Each{sum+=x};return sum}`,
	} {
		c := types2.NewContext()
		p, info, diagnostics, err := check(c, "example/p", source, "amd64", "go1.27", nil, false)
		if err != nil || len(diagnostics) != 0 || !p.Complete() {
			t.Fatalf("%s: %v %v", source, err, diagnostics)
		}
		if len(info.Defs) == 0 || len(info.Types) == 0 {
			t.Fatal("missing native type information")
		}
		var bridged *gcbridge.CheckedPackage = p
		if bridged != p {
			t.Fatal("copied package")
		}
		if strings.Contains(source, "Box[") {
			if len(info.Instances) != 2 {
				t.Fatalf("instances: %v", info.Instances)
			}
			x := p.Scope().Lookup("X").Type().Underlying().(*types2.Struct)
			if x.Field(0).Type() != c.Typ[types2.Int] {
				t.Fatal("generic result lost builtin identity")
			}
		}
	}
}

func TestTargetSizesVersionsAndDiagnostics(t *testing.T) {
	c := types2.NewContext()
	for _, tc := range []struct{ arch, want string }{{"386", "4"}, {"amd64", "8"}, {"386", "4"}} {
		p, _, _, err := check(c, "example/sizes", `package p;import "unsafe";const Word=unsafe.Sizeof(uintptr(0))`, tc.arch, "go1.27", importerFunc(func(path string) (*types2.Package, error) {
			if path == "unsafe" {
				return c.Unsafe, nil
			}
			return nil, fmt.Errorf("unexpected import %s", path)
		}), false)
		if err != nil {
			t.Fatal(err)
		}
		if got := p.Scope().Lookup("Word").(*types2.Const).Val().ExactString(); got != tc.want {
			t.Fatalf("%s: %s", tc.arch, got)
		}
	}
	_, _, ds, err := check(c, "example/old", `package p;func F[T any](x T)T{return x}`, "amd64", "go1.17", nil, false)
	if err == nil || len(ds) == 0 || !strings.Contains(err.Error(), "go1.18") {
		t.Fatalf("version: %v %v", err, ds)
	}
	_, _, ds, err = check(c, "example/bad", `package p;var X int="bad";func F()int{return Missing}`, "amd64", "go1.27", nil, false)
	if err == nil || len(ds) < 2 {
		t.Fatalf("diagnostics: %v %v", err, ds)
	}
	for _, err := range ds {
		d, ok := err.(types2.Error)
		if !ok || !d.Pos.IsKnown() || d.Code == 0 || d.Msg == "" {
			t.Fatalf("native diagnostic: %#v", err)
		}
	}
	if _, _, _, err := check(c, "example/after", `package p;var X=3`, "amd64", "go1.27", nil, false); err != nil {
		t.Fatalf("failed check poisoned context: %v", err)
	}
}

func TestImportsAndRetainedGraphs(t *testing.T) {
	c := types2.NewContext()
	imported, _, _, err := check(c, "example/q", `package q;type N int;const K=3`, "amd64", "go1.27", nil, false)
	if err != nil {
		t.Fatal(err)
	}
	loader := importerFunc(func(path string) (*types2.Package, error) {
		if path == "example/q" {
			return imported, nil
		}
		return nil, fmt.Errorf("unexpected import %s", path)
	})
	p, _, _, err := check(c, "example/p", `package p;import "example/q";var X=q.N(q.K)`, "amd64", "go1.27", loader, false)
	if err != nil {
		t.Fatal(err)
	}
	saved := p.Scope().Lookup("X").Type()
	if saved != imported.Scope().Lookup("N").Type() {
		t.Fatal("import identity")
	}
	if _, _, _, err := check(c, "example/again", `package p;var X=5`, "amd64", "go1.27", nil, false); err != nil {
		t.Fatal(err)
	}
	other := types2.NewContext()
	other.Universe.Insert(other.NewVar(syntax.Pos{}, nil, "OnlyInOther", other.Typ[types2.Int]))
	if c.Universe.Lookup("OnlyInOther") != nil || p.Scope().Lookup("X").Type() != saved || saved.String() != "example/q.N" {
		t.Fatal("retained graph or universe isolation")
	}
	defer func() {
		if recover() == nil {
			t.Fatal("foreign import graph accepted")
		}
	}()
	check(other, "example/foreign", `package p;import "example/q";var X=q.N(1)`, "amd64", "go1.27", loader, false)
}

func TestConcurrentChecksAndTraceIsolation(t *testing.T) {
	var wg sync.WaitGroup
	for i := range 16 {
		wg.Go(func() {
			c := types2.NewContext()
			var trace bytes.Buffer
			c.TraceWriter = &trace
			p, _, _, err := check(c, fmt.Sprintf("example/p%d", i), `package p;func Id[T any](x T)T{return x};var X=Id(3)`, "amd64", "go1.27", nil, true)
			if err != nil {
				t.Error(err)
				return
			}
			if p.Scope().Lookup("X").Type() != c.Typ[types2.Int] || trace.Len() == 0 {
				t.Error("type identity or missing trace")
			}
		})
	}
	wg.Wait()
}

func TestEmptyTupleFormatting(t *testing.T) {
	if got := (*types2.Tuple)(nil).String(); got != "()" {
		t.Fatal(got)
	}
}

func TestReentrantDiagnosticCallback(t *testing.T) {
	outer, inner := types2.NewContext(), types2.NewContext()
	f, err := syntax.Parse(syntax.NewFileBase("outer.go"), strings.NewReader(`package p;var X int="bad"`), nil, nil, syntax.CheckBranches)
	if err != nil {
		t.Fatal(err)
	}
	var nested *types2.Package
	conf := types2.Config{Context: outer, GoVersion: "go1.27", Sizes: outer.SizesFor("gc", "amd64"), Error: func(error) {
		p, _, _, err := check(inner, "example/inner", `package p;func Id[T any](x T)T{return x};var X=Id(7)`, "amd64", "go1.27", nil, false)
		if err != nil {
			t.Error(err)
		}
		nested = p
	}}
	if _, err := conf.Check("example/outer", []*syntax.File{f}, nil); err == nil {
		t.Fatal("outer error lost")
	}
	if nested == nil || nested.Scope().Lookup("X").Type() != inner.Typ[types2.Int] || inner.Typ[types2.Int] == outer.Typ[types2.Int] {
		t.Fatal("nested check did not retain its own graph")
	}
}

func TestIndependentInstantiationCachesKeepTypeUniverse(t *testing.T) {
	cache := types2.NewContext()
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			owner := types2.NewContext()
			p, _, _, e := check(owner, "example/cache", `package p;type Box[T any]struct{V T};type View[T any]=Box[T];func Identity[T any](v T)T{return v};type HasM interface{M()}`, "amd64", "go1.27", nil, false)
			if e != nil {
				t.Error(e)
				return
			}
			arg := p.Scope().Lookup("HasM").Type().Underlying()
			for _, name := range []string{"Box", "View", "Identity"} {
				orig := p.Scope().Lookup(name).Type()
				first, e := owner.Instantiate(cache, orig, []types2.Type{arg}, true)
				if e != nil {
					t.Error(e)
					continue
				}
				second, e := owner.Instantiate(cache, orig, []types2.Type{arg}, true)
				if e != nil || !owner.Identical(first, second) {
					t.Error("independent cache changed type identity", e)
				}
				_ = first.Underlying()
				_ = first.String()
			}
		})
	}
	wg.Wait()
}

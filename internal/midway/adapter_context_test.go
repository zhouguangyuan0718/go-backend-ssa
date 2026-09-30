package midway

import (
	"fmt"
	"go-backend-ssa/internal/base"
	"go-backend-ssa/internal/go/buildcfg"
	"go-backend-ssa/internal/syntax"
	"go-backend-ssa/internal/types2"
	"strings"
	"testing"
)

type testImports map[string]*types2.Package

func (m testImports) Import(path string) (*types2.Package, error) {
	p := m[path]
	if p == nil {
		return nil, fmt.Errorf("unknown import %s", path)
	}
	return p, nil
}
func midwayInput(t *testing.T, arch string) (*Context, *types2.Package, *types2.Info, []*syntax.File) {
	t.Helper()
	checker := types2.NewContext()
	c := NewContext(checker, arch, buildcfg.ExperimentFlags{}, base.DebugFlags{})
	c.Experiment.SIMD = true
	info := &types2.Info{StoreTypesInSyntax: true, Defs: map[*syntax.Name]types2.Object{}, Uses: map[*syntax.Name]types2.Object{}}
	parse := func(name, source string) *syntax.File {
		f, e := syntax.Parse(syntax.NewFileBase(name), strings.NewReader(source), nil, nil, syntax.CheckBranches)
		if e != nil {
			t.Fatal(e)
		}
		return f
	}
	conf := types2.Config{Context: checker, GoVersion: "go1.27"}
	simd, e := conf.Check("simd", []*syntax.File{parse("vector.go", `package simd;type Int8s [16]int8;func VectorBitSize()int{return 128};func Emulated()bool{return false}`)}, nil)
	if e != nil {
		t.Fatal(e)
	}
	conf.Importer = testImports{"simd": simd}
	files := []*syntax.File{parse("input.go", `package p;import "simd";type Node struct{Next *Node;V simd.Int8s};var Global simd.Int8s;func Identity(v simd.Int8s)simd.Int8s{var x simd.Int8s=v;return x};func Count()int{v:=Global;return len(v)}`)}
	pkg, e := conf.Check("p", files, info)
	if e != nil {
		t.Fatal(e)
	}
	return c, pkg, info, files
}
func requirePanic(t *testing.T, fn func()) {
	t.Helper()
	defer func() {
		if recover() == nil {
			t.Error("missing panic")
		}
	}()
	fn()
}
func TestConcurrentMidwayAndRetainedObjects(t *testing.T) {
	for i := range 12 {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			t.Parallel()
			arch := []string{"amd64", "arm64", "wasm"}[i%3]
			c, pkg, info, files := midwayInput(t, arch)
			old := pkg.Scope().Lookup("Identity")
			c.Debug.Simd = 1
			var warnings []string
			c.Warn = func(f string, a ...any) { warnings = append(warnings, fmt.Sprintf(f, a...)) }
			if !c.RewriteWrapper(pkg, info, files) {
				t.Fatal("did not rewrite")
			}
			sizes := []int{0, 128}
			if arch == "amd64" {
				sizes = append(sizes, 256, 512)
			}
			found := map[string]*syntax.FuncDecl{}
			for _, d := range files[0].DeclList {
				if fn, ok := d.(*syntax.FuncDecl); ok {
					found[fn.Name.Value] = fn
				}
			}
			for _, n := range sizes {
				key := fmt.Sprintf("Identity@simd%d", n)
				fn := found[key]
				if fn == nil {
					t.Fatalf("missing %s", key)
				}
				o := info.Defs[fn.Name]
				if o == nil || o == old || o.Pkg() != pkg {
					t.Fatalf("copied object lost ownership: %v", o)
				}
				decl := fn.Body.List[0].(*syntax.DeclStmt).DeclList[0].(*syntax.VarDecl)
				def := decl.NameList[0]
				ret := fn.Body.List[1].(*syntax.ReturnStmt).Results.(*syntax.Name)
				if info.Defs[def] == nil || info.Uses[ret] != info.Defs[def] {
					t.Fatal("copied local uses do not map to copied variable")
				}
			}
			if len(warnings) == 0 {
				t.Fatal("debug warnings were not routed")
			}
			// Construct and use another compilation while retaining the first graph.
			other, q, qi, qf := midwayInput(t, "amd64")
			other.RewriteWrapper(q, qi, qf)
			if pkg.Scope().Lookup("Identity") != old || old.Pkg() != pkg {
				t.Fatal("new compilation changed retained graph")
			}
			c.CheckPositions(files, "retained")
		})
	}
}
func TestMidwayOwnershipBoundaries(t *testing.T) {
	c, pkg, info, files := midwayInput(t, "amd64")
	other, q, qi, _ := midwayInput(t, "arm64")
	a := c.NewAnalyzer(pkg, info)
	a.Analyze(files)
	requirePanic(t, func() { other.NewAnalyzer(pkg, info) })
	requirePanic(t, func() { c.NewAnalyzer(q, qi) })
	requirePanic(t, func() { c.NewRewriter(pkg, &types2.Info{}, a, []int{128}) })
	requirePanic(t, func() { other.NewDeepCopier(q, qi, 128, a, "@simd128") })
	requirePanic(t, func() { NewContext(nil, "amd64", buildcfg.ExperimentFlags{}, base.DebugFlags{}) })
	requirePanic(t, func() { c.CheckPositions([]*syntax.File{{}}, "invalid") })
}
func TestMidwayDisabledDoesNotMutate(t *testing.T) {
	for _, arch := range []string{"amd64", "386"} {
		c, pkg, info, files := midwayInput(t, arch)
		if arch == "amd64" {
			c.Experiment.SIMD = false
		}
		before := append([]syntax.Decl(nil), files[0].DeclList...)
		if c.RewriteWrapper(pkg, info, files) {
			t.Fatal("disabled rewrite ran")
		}
		for i, d := range before {
			if files[0].DeclList[i] != d {
				t.Fatal("disabled rewrite changed declarations")
			}
		}
	}
}

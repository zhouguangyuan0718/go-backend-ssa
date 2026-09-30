package noder

import (
	"fmt"
	"go-backend-ssa/internal/bloop"
	"go-backend-ssa/internal/cmd/cov/covcmd"
	"go-backend-ssa/internal/cmd/objabi"
	"go-backend-ssa/internal/coverage"
	"go-backend-ssa/internal/ir"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const benchmarkDependency = `package testing;type B struct{N int};func(b *B)Loop()bool{b.N--;return b.N>0};func(b *B)More()bool{return b.Loop()}`
const benchmarkSource = `package p
import "testing"
func Result(x int)int{return x+1}
func Pair()(int,int){return 1,2}
func Void(x int, xs ...int){}
func Benchmark(b *testing.B){
 for b.Loop(){
  x:=1;x++;_ = Result(x);_,y:=Pair();x,y=y,x;Result(x);Pair();Void(x,Result(y),3)
  if x>0{x=2}else{x=3}
  for i:=0;i<2;i++{x+=i}
  for _,v:=range []int{1,2}{x=v}
  switch x{case 0:x=1;default:x=2}
  select{default:x=4}
 }
}
func Ordinary(b *testing.B){for b.More(){Result(1);Void(1)}}
func Outside(){Result(1);Void(1)}
`

func benchmarkArchive(t *testing.T) []byte {
	t.Helper()
	dir := t.TempDir()
	file := filepath.Join(dir, "testing.go")
	archive := file + ".a"
	if e := os.WriteFile(file, []byte(benchmarkDependency), 0600); e != nil {
		t.Fatal(e)
	}
	cmd := exec.Command("go", "tool", "compile", "-p", "testing", "-pack", "-o", archive, file)
	cmd.Env = append(os.Environ(), "GOWORK=off", "GOTOOLCHAIN=go1.27.1", "GOOS=linux", "GOARCH=amd64", "GOAMD64=v1", "GOEXPERIMENT=none")
	if b, e := cmd.CombinedOutput(); e != nil {
		t.Fatalf("%v\n%s", e, b)
	}
	b, e := os.ReadFile(archive)
	if e != nil {
		t.Fatal(e)
	}
	return b
}
func TestNativeBenchmarkLiveness(t *testing.T) {
	archive := benchmarkArchive(t)
	for i := range 8 {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			t.Parallel()
			c, _, _ := dependencyContext(t, map[string][]byte{"testing": archive}, false)
			c.Flag.Cfg.PackageFile = map[string]string{"testing": "testing"}
			f := parseSource(t, c, "benchmark.go", benchmarkSource)
			pkg, e := c.CompileParsedPackage([]*ParsedFile{f})
			if e != nil {
				t.Fatal(e)
			}
			c.Flag.LowerM = 2
			pass := bloop.NewContext(c.Typecheck)
			pass.Walk(pkg)
			kept := 0
			for _, fn := range pkg.Funcs {
				ir.Visit(fn, func(n ir.Node) {
					if call, ok := n.(*ir.CallExpr); ok && call.IsCompilerVarLive {
						kept++
						if ir.FuncName(fn) != "Benchmark" || !call.NoInline || call.Typecheck() != 1 || c.Typecheck.IR.StaticCalleeName(call.Fun).Sym().Name != "KeepAlive" {
							t.Fatalf("unexpected live call: %v in %s", call, ir.FuncName(fn))
						}
					}
				})
			}
			if kept < 20 {
				t.Fatalf("missing liveness rewrites: %d", kept)
			}
			if len(c.Typecheck.IR.Diagnostics.Diagnostics()) == 0 {
				t.Fatal("missing instance diagnostics")
			}
			// A foreign member appended at the end must be rejected before any earlier
			// function can be rewritten again.
			foreign := bodyContext(t)
			initializeTestPackage(t, foreign, `package p;func Alien(){}`)
			pkg.Funcs = append(pkg.Funcs, foreign.Typecheck.Target.Funcs[0])
			before := len(pkg.Funcs[0].Dcl)
			mustPanicPrepackage(t, func() { pass.Walk(pkg) })
			if len(pkg.Funcs[0].Dcl) != before {
				t.Fatal("mutated before owner validation")
			}
			mustPanicPrepackage(t, func() { pass.Walk(foreign.Typecheck.Target) })
		})
	}
}
func mustPanicPrepackage(t *testing.T, f func()) {
	t.Helper()
	defer func() {
		if recover() == nil {
			t.Error("expected rejection")
		}
	}()
	f()
}

const coverSource = `package p;var Meta=[4]byte{1,2,3,4};var PkgID uint32;var Counter0 [8]uint32;var Ordinary int;func init(){Ordinary=3};func F(){Counter0[0]++}`

func TestNativeCoverageFixup(t *testing.T) {
	for _, path := range []string{"example/p", "main", "runtime"} {
		for _, mode := range []string{"set", "count", "atomic", "regonly", "testmain"} {
			for _, gran := range []string{"perblock", "perfunc"} {
				t.Run(path+"/"+mode+"/"+gran, func(t *testing.T) {
					t.Parallel()
					c, _, _ := dependencyContext(t, nil, false)
					c.Ctxt.Pkgpath = path
					c.Types.LocalPkg = c.Types.NewPkg(path, "")
					src := coverSource
					if path == "main" {
						src = strings.Replace(src, "package p", "package main", 1)
					}
					f := parseSource(t, c, "cover.go", src)
					pkg, e := c.CompileParsedPackage([]*ParsedFile{f})
					if e != nil {
						t.Fatal(e)
					}
					config := &covcmd.CoverFixupConfig{MetaVar: "Meta", PkgIdVar: "PkgID", CounterPrefix: "Counter", CounterMode: mode, CounterGranularity: gran, MetaHash: "0123456789abcdef0123456789abcdef", MetaLen: 4}
					c.Flag.Cfg.CoverageInfo = config
					coverage.NewContext(c.Typecheck).Fixup()
					names := map[string]*ir.Name{}
					for _, n := range pkg.Externs {
						names[n.Sym().Name] = n
					}
					if !names["Meta"].Readonly() || !names["PkgID"].CoverageAuxVar() || names["PkgID"].Linksym().Type != objabi.SCOVERAGE_AUXVAR || names["Counter0"].Linksym().Type != objabi.SCOVERAGE_COUNTER || names["Ordinary"].CoverageAuxVar() {
						t.Fatal("incorrect coverage classification")
					}
					var init *ir.Func
					for _, fn := range pkg.Funcs {
						if ir.FuncName(fn) == "init" {
							init = fn
						}
					}
					if init == nil {
						t.Fatal("missing init")
					}
					calls := []*ir.CallExpr{}
					ir.Visit(init, func(n ir.Node) {
						if call, ok := n.(*ir.CallExpr); ok {
							calls = append(calls, call)
						}
					})
					want := 0
					if mode != "testmain" {
						want++
					}
					if path == "main" {
						want++
					}
					if len(calls) != want {
						t.Fatalf("calls: %d want %d", len(calls), want)
					}
					if mode != "testmain" {
						call := calls[0]
						if c.Typecheck.IR.StaticCalleeName(call.Fun).Sym().Name != "addCovMeta" || len(call.Args) != 7 || c.Typecheck.IR.StringVal(call.Args[3]) != path {
							t.Fatal("bad registration", call)
						}
						if (init.Body[0].Op() == ir.OAS) != (path != "runtime") {
							t.Fatal("hard-coded package ID assignment")
						}
						if c.Typecheck.IR.IntVal(call.Args[1].Type(), call.Args[1].Val()) != 4 {
							t.Fatal("wrong metadata length")
						}
					}
					if path == "main" {
						call := calls[len(calls)-1]
						if c.Typecheck.IR.StaticCalleeName(call.Fun).Sym().Name != "initHook" || c.Typecheck.IR.BoolVal(call.Args[0]) != (mode == "testmain") {
							t.Fatal("bad init hook", call)
						}
						if init.Body[len(init.Body)-1] != call {
							t.Fatal("hook not last")
						}
					}
				})
			}
		}
	}
}
func TestCoveragePreconditions(t *testing.T) {
	mustPanicPrepackage(t, func() { coverage.NewContext(nil) })
	mustPanicPrepackage(t, func() { bloop.NewContext(nil) })
	for _, change := range []func(*covcmd.CoverFixupConfig){func(c *covcmd.CoverFixupConfig) { c.CounterMode = "bad" }, func(c *covcmd.CoverFixupConfig) { c.CounterGranularity = "bad" }, func(c *covcmd.CoverFixupConfig) { c.MetaHash = "short" }, func(c *covcmd.CoverFixupConfig) { c.MetaHash = strings.Repeat("zz", 16) }} {
		c, _, _ := dependencyContext(t, nil, false)
		f := parseSource(t, c, "cover.go", coverSource)
		if _, e := c.CompileParsedPackage([]*ParsedFile{f}); e != nil {
			t.Fatal(e)
		}
		pass := coverage.NewContext(c.Typecheck)
		pass.Fixup() // no config is the native no-op
		cfg := &covcmd.CoverFixupConfig{MetaVar: "Meta", PkgIdVar: "PkgID", CounterPrefix: "Counter", CounterMode: "set", CounterGranularity: "perblock", MetaHash: strings.Repeat("00", 16), MetaLen: 4}
		change(cfg)
		c.Flag.Cfg.CoverageInfo = cfg
		mustPanicPrepackage(t, pass.Fixup)
	}
}

func TestCoverageRejectsForeignGraphBeforeMutation(t *testing.T) {
	c, _, _ := dependencyContext(t, nil, false)
	f := parseSource(t, c, "cover.go", coverSource)
	if _, e := c.CompileParsedPackage([]*ParsedFile{f}); e != nil {
		t.Fatal(e)
	}
	c.Flag.Cfg.CoverageInfo = &covcmd.CoverFixupConfig{MetaVar: "Meta", PkgIdVar: "PkgID", CounterPrefix: "Counter", CounterMode: "set", CounterGranularity: "perblock", MetaHash: strings.Repeat("00", 16), MetaLen: 4}
	meta := c.Types.LocalPkg.Lookup("Meta").Def.(*ir.Name)
	foreign := bodyContext(t)
	initializeTestPackage(t, foreign, `package p;var Alien int;func F(){}`)
	pass := coverage.NewContext(c.Typecheck)
	funcs := c.Typecheck.Target.Funcs
	c.Typecheck.Target.Funcs = append(c.Typecheck.Target.Funcs, foreign.Typecheck.Target.Funcs[0])
	mustPanicPrepackage(t, pass.Fixup)
	if meta.Readonly() {
		t.Fatal("metadata mutated before foreign function rejection")
	}
	c.Typecheck.Target.Funcs = funcs
	c.Typecheck.Target.Externs = append(c.Typecheck.Target.Externs, foreign.Typecheck.Target.Externs[0])
	mustPanicPrepackage(t, pass.Fixup)
	if meta.Readonly() {
		t.Fatal("metadata mutated before foreign variable rejection")
	}
}

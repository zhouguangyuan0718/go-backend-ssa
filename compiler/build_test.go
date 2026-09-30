package compiler_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"go-backend-ssa/compiler"
	"go-backend-ssa/gcbridge"
	"go-backend-ssa/ssa"
	"io/fs"
	"os"
	"strings"
	"testing"
	"testing/fstest"
)

func builtText(p *compiler.Package) string {
	var out strings.Builder
	fmt.Fprintf(&out, "%s:%s:%s:%s\n", p.Name, p.ImportPath, p.Metadata.Stage, p.Metadata.Experiments)
	for _, f := range p.Functions {
		out.WriteString(f.String())
	}
	for _, s := range p.Globals {
		fmt.Fprintf(&out, "%s:%d:%d:%d:%x\n", s.Name, s.Type, s.Size, s.Align, s.P)
		for _, r := range s.R {
			fmt.Fprintf(&out, "%d:%d:%d:%s:%d\n", r.Type, r.Off, r.Siz, r.Sym.Name, r.Add)
		}
	}
	return out.String()
}
func TestSourceBuildRetainsCompletePackage(t *testing.T) {
	for i := range 8 {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			t.Parallel()
			depReq := request("example/dep", `package dep
var X=1
func init(){X++}
func Add[T ~int](a,b T)T{return a+b}
`)
			dep, err := compiler.Compile(context.Background(), depReq)
			if err != nil {
				t.Fatal(err)
			}
			req := request("example/p", `package p
import "example/dep"
type V struct{A int}
func(v V)Value()int{return v.A}
var M=map[string]int{"hello":3}
var I any=V{4}
func init(){M["hello"]++}
func F(n int)int{s:=0;for i:=range n{if i&1==0{s+=i}else{s-=i}};return dep.Add(s,1)}
func Closure(n int)func()int{return func()int{return n+M["hello"]}}
func Defer(p *int){defer func(){*p+=2}();defer func(){*p+=3}()}
`)
			req.Imports = map[string][]byte{"example/dep": dep.Archive}
			req.Options.Workers = 4
			req.Options.DisableInlining = i%2 != 0
			p, err := compiler.Build(context.Background(), req)
			if err != nil {
				t.Fatal(err)
			}
			if p.Metadata.Stage != compiler.Built || p.Metadata.UpstreamCommit != compiler.UpstreamCommit || p.Metadata.Language != "go1.27" || p.Types.LocalPkg.Path != req.ImportPath || p.Positions == nil || p.Declarations == nil {
				t.Fatal("missing owned package metadata")
			}
			if len(p.Functions) < 6 || len(p.RuntimeTypes) == 0 || len(p.Globals) == 0 || len(p.Symbols) == 0 || p.Init.Task == nil || len(p.Init.Dependencies) != 1 || p.Init.Dependencies[0].ImportPath != "example/dep" {
				t.Fatal("incomplete package fixed point", len(p.Functions), len(p.RuntimeTypes), p.Init)
			}
			if len(p.Imports) != 1 || p.Imports[0].ArchiveSHA256 != sha256.Sum256(dep.Archive) {
				t.Fatal("missing input identity")
			}
			symbols := map[*gcbridge.LinkSymbol]bool{}
			for _, sym := range p.Symbols {
				symbols[sym] = true
			}
			for _, sym := range p.Globals {
				if !symbols[sym] {
					t.Fatal("global missing from symbols", sym.Name)
				}
				for _, r := range sym.R {
					if r.Sym != nil && !symbols[r.Sym] {
						t.Fatal("relocation target missing", r.Sym.Name)
					}
				}
			}
			var initFunctions []*gcbridge.LinkSymbol
			for _, r := range p.Init.Task.R {
				if r.Type == gcbridge.R_ADDR {
					initFunctions = append(initFunctions, r.Sym)
				}
			}
			if len(initFunctions) != len(p.Init.Functions) || len(initFunctions) == 0 {
				t.Fatal("initializer function count")
			}
			for i, fn := range p.Init.Functions {
				if fn.LSym != initFunctions[i] {
					t.Fatal("initializer order or identity")
				}
			}
			seen := map[*ssa.Cache]bool{}
			closure, wrapper, openDefer := false, false, false
			for _, f := range p.Functions {
				if f.RegAlloc != nil || len(f.RegArgs) != 0 || len(f.Frontend().Func().LSym.P) != 0 || seen[f.Cache] {
					t.Fatal("native tail ran or graph shares cache", f.Name)
				}
				seen[f.Cache] = true
				closure = closure || f.Frontend().Func().IsClosure()
				wrapper = wrapper || strings.Contains(f.Name, "(*V).Value")
				for _, b := range f.Blocks {
					for _, v := range b.Values {
						if v.Op == ssa.OpFwdRef {
							t.Fatal("incomplete Phi construction")
						}
						if v.Type != nil && v.Type.Universe() != p.Types {
							t.Fatal("foreign value type")
						}
					}
				}
				if d, ok := p.OpenDefers[f]; ok {
					if d.Bits == nil || len(d.Closures) != 2 {
						t.Fatal("missing defer slots")
					}
					openDefer = true
				}
			}
			if !closure || !wrapper || !openDefer {
				t.Fatal("missing generated functions or open-defer metadata", closure, wrapper, openDefer)
			}
			for _, rt := range p.RuntimeTypes {
				if rt.Type.Universe() != p.Types || rt.Symbol == nil {
					t.Fatal("foreign runtime type")
				}
			}
			before := builtText(p)
			repeat, err := compiler.Build(context.Background(), req)
			if err != nil || builtText(repeat) != before {
				t.Fatal("nondeterministic Built result", err)
			}
			clear(dep.Archive)
			clear(req.Files[0].Data)
			other, err := compiler.Build(context.Background(), request("example/p", "package p;func Other()int{return 99}"))
			if err != nil {
				t.Fatal(err)
			}
			if builtText(p) != before || other.Types == p.Types || other.Positions == p.Positions {
				t.Fatal("later build changed retained package")
			}
		})
	}
}

func TestBuildErrorsHaveNoResult(t *testing.T) {
	for name, req := range map[string]compiler.Request{
		"source":   request("p", "package p;func F()int{return missing}"),
		"target":   {ImportPath: "p", Files: []compiler.SourceFile{{Name: "p.go", Data: []byte("package p")}}},
		"stage":    func() compiler.Request { r := request("p", "package p"); r.Options.Stage = "lowered"; return r }(),
		"assembly": func() compiler.Request { r := request("p", "package p"); r.Options.AssemblyHeader = true; return r }(),
	} {
		t.Run(name, func(t *testing.T) {
			p, err := compiler.Build(context.Background(), req)
			if p != nil || err == nil {
				t.Fatal("invalid result", p, err)
			}
		})
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if p, err := compiler.Build(ctx, request("p", "package p")); p != nil || !errors.Is(err, context.Canceled) {
		t.Fatal(p, err)
	}
	req := request("p", "package p")
	req.Options.Stage = compiler.Built
	if p, err := compiler.Compile(context.Background(), req); p != nil || err == nil {
		t.Fatal("native compile accepted SSA stage")
	}
	empty, err := compiler.Build(context.Background(), req)
	if err != nil || len(empty.Functions) != 0 {
		t.Fatal("empty package", empty, err)
	}
	good, err := compiler.Compile(context.Background(), request("p", "package p;func F()int{return 7}"))
	if err != nil || !bytes.HasPrefix(good.Archive, []byte("!<arch>\n")) {
		t.Fatal(err)
	}
}

func TestSourceBuildEmbeddedInput(t *testing.T) {
	dep, err := compiler.Compile(context.Background(), request("embed", "package embed"))
	if err != nil {
		t.Fatal(err)
	}
	req := request("p", "package p\nimport _ \"embed\"\n//go:embed data\nvar Data string\nfunc Len()int{return len(Data)}")
	req.Imports = map[string][]byte{"embed": dep.Archive}
	req.Embed.Patterns = map[string][]string{"data": {"data"}}
	req.Embed.Files = map[string]string{"data": "data"}
	data := []byte(strings.Repeat("owned embedded contents", 200))
	files := fstest.MapFS{"data": &fstest.MapFile{Data: data}}
	req.Embed.Open = files.Open
	p, err := compiler.Build(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, sym := range p.Globals {
		found = found || bytes.Equal(sym.P, data)
	}
	if !found {
		t.Fatal("embedded data missing")
	}
	before := builtText(p)
	clear(data)
	if builtText(p) != before {
		t.Fatal("result retained provider bytes")
	}
	req.Embed.Open = func(string) (fs.File, error) { return nil, fs.ErrNotExist }
	if p, err := compiler.Build(context.Background(), req); p != nil || err == nil {
		t.Fatal("missing embedded input accepted")
	}
	for _, mode := range []string{"missing", "truncated"} {
		calls := 0
		req.Embed.Open = func(name string) (fs.File, error) {
			calls++
			if calls == 1 {
				return files.Open(name)
			}
			if mode == "missing" {
				return nil, fs.ErrNotExist
			}
			return (fstest.MapFS{"data": &fstest.MapFile{Data: []byte("short")}}).Open(name)
		}
		if p, err := compiler.Build(context.Background(), req); p != nil || err == nil || calls != 2 {
			t.Fatal("deferred embedded input failure accepted", mode, calls, err)
		}
	}
	marker := new(int)
	req.Embed.Open = func(string) (fs.File, error) { panic(marker) }
	func() {
		defer func() {
			if recover() != marker {
				t.Error("provider panic changed")
			}
		}()
		_, _ = compiler.Build(context.Background(), req)
		t.Error("provider panic swallowed")
	}()
	ctx, cancel := context.WithCancel(context.Background())
	req.Embed.Open = func(name string) (fs.File, error) { cancel(); return files.Open(name) }
	if p, err := compiler.Build(ctx, req); p != nil || !errors.Is(err, context.Canceled) {
		t.Fatal("cancellation returned output", err)
	}
}

func TestSourceBuildProfileInputs(t *testing.T) {
	source, err := os.ReadFile("../testdata/pgo/hot.go")
	if err != nil {
		t.Fatal(err)
	}
	req := request("p", string(source))
	req.Files[0].Name = "hot.go"
	cold, err := compiler.Build(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	var expected string
	for _, name := range []string{"hot.pb", "hot.pprof", "hot.preprofile"} {
		data, err := os.ReadFile("../testdata/pgo/" + name)
		if err != nil {
			t.Fatal(err)
		}
		req.PGOProfile = &compiler.ProfileInput{Name: name, Data: data}
		p, err := compiler.Build(context.Background(), req)
		if err != nil {
			t.Fatal(err)
		}
		actual := builtText(p)
		if actual == builtText(cold) {
			t.Fatal("profile did not affect source build", name)
		}
		if expected == "" {
			expected = actual
		} else if actual != expected {
			t.Fatal("profile representations differ", name)
		}
		clear(data)
		if builtText(p) != expected {
			t.Fatal("result retained profile input")
		}
	}
	req.PGOProfile = &compiler.ProfileInput{Name: "empty.pprof"}
	empty, err := compiler.Build(context.Background(), req)
	if err != nil || builtText(empty) != builtText(cold) {
		t.Fatal("empty profile", err)
	}
	req.PGOProfile = &compiler.ProfileInput{Name: "bad.pprof", Data: []byte("not a profile")}
	if p, err := compiler.Build(context.Background(), req); p != nil || err == nil {
		t.Fatal("bad profile accepted")
	}
}

func TestSourceEarlyOptimizedPackage(t *testing.T) {
	for _, noopt := range []bool{false, true} {
		t.Run(fmt.Sprint(noopt), func(t *testing.T) {
			t.Parallel()
			req := request("example/early", `package early
func Add[T ~int](a,b T)T{return a+b}
var M=map[string]int{"x":3}
func F(n int)int{s:=n*0;for i:=range n{s+=i};return Add(s,1)}
func Defer(p *int){defer func(){*p+=3}()}
`)
			req.Options.DisableOptimization = noopt
			built, err := compiler.Build(context.Background(), req)
			if err != nil {
				t.Fatal(err)
			}
			before := builtText(built)
			req.Options.Stage = compiler.EarlyOptimized
			p, err := compiler.Build(context.Background(), req)
			if err != nil {
				t.Fatal(err)
			}
			if p.Metadata.Stage != compiler.EarlyOptimized || len(p.Functions) != len(built.Functions) || (!noopt && len(p.OpenDefers) == 0) {
				t.Fatal("early package metadata")
			}
			if !bytes.Equal(p.Artifact.UnifiedIR(), built.Artifact.UnifiedIR()) {
				t.Fatal("SSA optimization changed frontend export")
			}
			if _, err := compiler.DecodeArtifact(p.Artifact.Bytes()); err != nil {
				t.Fatal(err)
			}
			if before != builtText(built) {
				t.Fatal("later build changed retained graph")
			}
			changed := false
			for i, f := range p.Functions {
				if f.RegAlloc != nil || len(f.RegArgs) != 0 || len(f.Frontend().Func().LSym.P) != 0 {
					t.Fatal("early build ran native tail")
				}
				changed = changed || f.String() != built.Functions[i].String()
			}
			if !changed {
				t.Fatal("early optimization did not run")
			}
			consumer := request("example/consumer", `package consumer; import "example/early";func G()int{return early.Add(4,5)}`)
			consumer.Artifacts = map[string]*compiler.Artifact{"example/early": p.Artifact}
			if _, err := compiler.Build(context.Background(), consumer); err != nil {
				t.Fatal("early artifact import into Built", err)
			}
			consumer.Options.Stage = compiler.EarlyOptimized
			consumer.Artifacts["example/early"] = built.Artifact
			if _, err := compiler.Build(context.Background(), consumer); err != nil {
				t.Fatal("Built artifact import into EarlyOptimized", err)
			}
		})
	}
}

package backend

import (
	"bytes"
	"context"
	"fmt"
	"go-backend-ssa/compiler"
	"go-backend-ssa/gcbridge"
	"go-backend-ssa/ir"
	"go-backend-ssa/ssa"
	"go-backend-ssa/types"
	"path"
	"strings"
	"testing"
)

const source = `package sample
import "example/dep"
type Node struct{ Next *Node; Value int }
type Value interface{ Get() int }
func(n Node)Get()int{return n.Value}
var Global Value=Node{Value:7}
var Table=map[string]int{"x":3}
var Text="backend"
var Data=[3]int{2,5,9}
func init(){Table["x"]+=dep.N}
func Closure(x int)func()int{return func()int{return x+Table["x"]}}
func Interface(v Value)int{return v.Get()}
func Channels(ch chan int,n int)int{go func(){ch<-n}();return <-ch}
func Recover() (r int){defer func(){if recover()!=nil{r=7}}();panic("probe")}
func Defer(p *int){defer func(){*p+=2}();defer func(){*p+=3}()}
func Generic(x int)int{return dep.Add(x,4)}
func Scalar(n,seed int)int{if n<0{return seed-n};sum:=seed;for i:=0;i<n;i++{if i&1==0{sum+=i*3}else{sum-=i}};return sum}
`

func referenceScalar(n, seed int) int {
	if n < 0 {
		return seed - n
	}
	sum := seed
	for i := 0; i < n; i++ {
		if i&1 == 0 {
			sum += i * 3
		} else {
			sum -= i
		}
	}
	return sum
}

func TestSourceBackend(t *testing.T) {
	for _, arch := range []string{"amd64", "arm64"} {
		for _, stage := range []compiler.Stage{compiler.Built, compiler.EarlyOptimized} {
			t.Run(arch+"/"+string(stage), func(t *testing.T) {
				t.Parallel()
				target := compiler.Target{GOOS: "linux", GOARCH: arch}
				if arch == "amd64" {
					target.GOAMD64 = 1
				} else {
					target.GOARM64 = "v8.0"
				}
				dep, err := compiler.Build(context.Background(), compiler.Request{ImportPath: "example/dep", Target: target, Files: []compiler.SourceFile{{Name: "dep.go", Data: []byte(`package dep;var N=2;func init(){N++};func Add[T ~int](a,b T)T{return a+b}`)}}})
				if err != nil {
					t.Fatal(err)
				}
				artifact, err := compiler.DecodeArtifact(dep.Artifact.Bytes())
				if err != nil {
					t.Fatal(err)
				}
				req := compiler.Request{ImportPath: "example/sample", Target: target, Options: compiler.Options{Stage: stage}, Files: []compiler.SourceFile{{Name: "sample.go", Data: []byte(source)}}, Artifacts: map[string]*compiler.Artifact{"example/dep": artifact}}
				p, err := compiler.Build(context.Background(), req)
				if err != nil {
					t.Fatal(err)
				}
				scalar := inspectPackage(t, p)
				retained := scalar.String()
				savedArtifact := p.Artifact.Bytes()
				for _, n := range []int{-9, -1, 0, 1, 2, 3, 8, 31} {
					for _, seed := range []int{-7, 0, 11} {
						actual, err := interpret(scalar, map[string]int64{"n": int64(n), "seed": int64(seed)})
						if err != nil {
							t.Fatal(err)
						}
						if actual != int64(referenceScalar(n, seed)) {
							t.Fatalf("Scalar(%d,%d)=%d", n, seed, actual)
						}
					}
				}
				req.Files[0].Data = []byte("package sample;func Other()int{return 99}")
				if _, err := compiler.Build(context.Background(), req); err != nil {
					t.Fatal(err)
				}
				if scalar.String() != retained || !bytes.Equal(savedArtifact, p.Artifact.Bytes()) {
					t.Fatal("later compilation changed retained backend inputs")
				}
			})
		}
	}
}

// This consumer intentionally imports only public packages. It follows pointer
// identities through ABI descriptors, type data, relocations and defer slots.
func inspectPackage(t *testing.T, p *compiler.Package) *ssa.Func {
	t.Helper()
	symbols := map[*gcbridge.LinkSymbol]bool{}
	for _, s := range p.Symbols {
		symbols[s] = true
	}
	for _, s := range p.Globals {
		if !symbols[s] {
			t.Fatal("unowned global")
		}
		for _, r := range s.R {
			if r.Sym != nil && !symbols[r.Sym] {
				t.Fatal("unowned relocation", s.Name, r.Sym.Name)
			}
		}
	}
	if p.Init.Task == nil || len(p.Init.Functions) == 0 || len(p.Init.Dependencies) != 1 || p.Init.Dependencies[0].ImportPath != "example/dep" {
		t.Fatal("missing initialization graph")
	}
	for _, s := range p.Init.Functions {
		if !symbols[s.LSym] {
			t.Fatal("unowned initializer")
		}
	}
	if len(p.Imports) != 1 || p.Imports[0].ArtifactSHA256 == [32]byte{} {
		t.Fatal("missing dependency identity")
	}
	if len(p.RuntimeTypes) == 0 {
		t.Fatal("missing runtime types")
	}
	for _, rt := range p.RuntimeTypes {
		if rt.Type.Universe() != p.Types || !symbols[rt.Symbol] {
			t.Fatal("runtime type identity")
		}
	}
	calls, closures, interfaces, defers, positions := 0, 0, 0, 0, 0
	var scalar *ssa.Func
	for _, f := range p.Functions {
		if f.RegAlloc != nil {
			t.Fatal("unexpected native allocation")
		}
		if strings.HasSuffix(f.Frontend().Func().LSym.Name, ".Scalar") {
			scalar = f
		}
		fn := f.Frontend().Func()
		if fn.IsClosure() {
			closures++
		}
		if !symbols[fn.LSym] || f.OwnAux.ABI().Universe() != p.Types {
			t.Fatal("function ABI identity")
		}
		for _, b := range f.Blocks {
			for _, v := range b.Values {
				if v.Type != nil && v.Type.Universe() != p.Types {
					t.Fatal("foreign SSA type")
				}
				if v.Pos.IsKnown() {
					filename := path.Base(p.Positions.Pos(v.Pos).RelFilename())
					if filename != "sample.go" && filename != "dep.go" && filename != "<autogenerated>" {
						t.Fatal("unavailable source position", filename)
					}
					positions++
				}
				switch aux := v.Aux.(type) {
				case *ssa.AuxCall:
					calls++
					if aux.ABIInfo() == nil || aux.ABI().Universe() != p.Types {
						t.Fatal("call ABI owner")
					}
					for i := int64(0); i < aux.NArgs(); i++ {
						if aux.TypeOfArg(i).Universe() != p.Types {
							t.Fatal("call parameter type")
						}
					}
					for i := int64(0); i < aux.NResults(); i++ {
						if aux.TypeOfResult(i).Universe() != p.Types {
							t.Fatal("call result type")
						}
					}
				case *types.Type:
					if aux.Universe() != p.Types {
						t.Fatal("type Aux owner")
					}
				case *ir.Name:
					if aux.Type() != nil && aux.Type().Universe() != p.Types {
						t.Fatal("local Aux owner")
					}
				case *gcbridge.LinkSymbol:
					if !symbols[aux] {
						t.Fatal("symbol Aux missing from package", aux.Name)
					}
				}
				if v.Op == ssa.OpInterLECall || v.Op == ssa.OpInterCall {
					interfaces++
				}
			}
		}
		if d, ok := p.OpenDefers[f]; ok {
			if d.Bits == nil || len(d.Closures) == 0 {
				t.Fatal("missing open-defer slots")
			}
			locals := map[*ir.Name]bool{}
			for _, name := range fn.Dcl {
				locals[name] = true
			}
			if !locals[d.Bits] {
				t.Fatal("defer bit slot identity")
			}
			for _, slot := range d.Closures {
				if !locals[slot] {
					t.Fatal("defer closure slot identity")
				}
			}
			defers++
		}
	}
	if scalar == nil || calls == 0 || closures == 0 || interfaces == 0 || defers == 0 || positions == 0 {
		t.Fatal("incomplete backend fixture", calls, closures, interfaces, defers, positions)
	}
	t.Logf("%d functions, %d globals, %d runtime types, %d calls, %d open-defer functions", len(p.Functions), len(p.Globals), len(p.RuntimeTypes), calls, defers)
	return scalar
}

// interpret is a test-only backend for integer control flow. It resolves value
// dependencies without relying on unscheduled value-list order. Phi assignments
// are simultaneous, using the incoming edge's public predecessor index.
func interpret(f *ssa.Func, args map[string]int64) (int64, error) {
	values := map[*ssa.Value]int64{}
	var eval func(*ssa.Value) (int64, error)
	eval = func(v *ssa.Value) (int64, error) {
		if x, ok := values[v]; ok {
			return x, nil
		}
		var a, b int64
		var err error
		if len(v.Args) > 0 {
			a, err = eval(v.Args[0])
			if err != nil {
				return 0, err
			}
		}
		if len(v.Args) > 1 {
			b, err = eval(v.Args[1])
			if err != nil {
				return 0, err
			}
		}
		var result int64
		switch v.Op {
		case ssa.OpArg:
			n, ok := v.Aux.(*ir.Name)
			if !ok {
				return 0, fmt.Errorf("argument has unsupported Aux %T", v.Aux)
			}
			result, ok = args[n.Sym().Name]
			if !ok {
				return 0, fmt.Errorf("missing argument %s", n.Sym().Name)
			}
		case ssa.OpConst64, ssa.OpConstBool:
			result = v.AuxInt
		case ssa.OpInitMem:
			result = 0
		case ssa.OpCopy:
			result = a
		case ssa.OpAdd64:
			result = a + b
		case ssa.OpSub64:
			result = a - b
		case ssa.OpMul64:
			result = a * b
		case ssa.OpAnd64:
			result = a & b
		case ssa.OpNeg64:
			result = -a
		case ssa.OpLess64:
			if a < b {
				result = 1
			}
		case ssa.OpEq64:
			if a == b {
				result = 1
			}
		case ssa.OpNeq64:
			if a != b {
				result = 1
			}
		case ssa.OpMakeResult:
			result = a
		default:
			return 0, fmt.Errorf("test backend does not implement %s", v.LongString())
		}
		values[v] = result
		return result, nil
	}
	block, pred := f.Entry, -1
	for steps := 0; steps < 10000; steps++ {
		phis := map[*ssa.Value]int64{}
		for _, v := range block.Values {
			if v.Op == ssa.OpPhi {
				if pred < 0 || pred >= len(v.Args) {
					return 0, fmt.Errorf("invalid Phi input")
				}
				x, err := eval(v.Args[pred])
				if err != nil {
					return 0, err
				}
				phis[v] = x
			}
		}
		for _, v := range block.Values {
			delete(values, v)
		}
		for v, x := range phis {
			values[v] = x
		}
		edge := 0
		switch block.Kind {
		case ssa.BlockPlain:
		case ssa.BlockIf:
			c, err := eval(block.Controls[0])
			if err != nil {
				return 0, err
			}
			if c == 0 {
				edge = 1
			}
		case ssa.BlockRet:
			return eval(block.Controls[0])
		default:
			return 0, fmt.Errorf("test backend does not implement block %s", block.Kind)
		}
		next := block.Succs[edge]
		block, pred = next.Block(), next.Index()
	}
	return 0, fmt.Errorf("test backend exceeded step bound")
}

package amd64

import (
	"fmt"
	"go-backend-ssa/internal/ssa"
	"go-backend-ssa/internal/ssagen"
	"slices"
	"testing"
)

func TestRetainedSSAConstruction(t *testing.T) {
	for i := range 8 {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			t.Parallel()
			c, abis := loadQueueCompiler(t, 4, `package p
func F(n int)int{s:=0;for i:=range n{if i&1==0{s+=i}else{s-=i}};return s}
func G(p *int,n int)*int{*p+=n;return p}
func H(n int)int{return n*3+1}
`, true)
			c.PrepareFrontend(abis, nil)
			backend := c.Backend
			functions := c.Package.Noder.Typecheck.Target.Funcs
			if len(functions) != 3 {
				t.Fatal("unexpected function count", len(functions))
			}
			for _, fn := range functions {
				c.EnqueueFunc(fn, abis)
			}
			saved := make([]*ssagen.Construction, 2)
			text := make([]string, 2)
			for j := range saved {
				saved[j] = backend.ConstructSSA(functions[j], j == 0)
				if backend.WorkerCount() != 4 {
					t.Fatal("native worker count changed")
				}
				f := saved[j].Func()
				if f.RegAlloc != nil || len(f.RegArgs) != 0 || saved[j].NativeCompleted() {
					t.Fatal("construction ran native tail")
				}
				for _, b := range f.Blocks {
					for _, v := range b.Values {
						if v.Op == ssa.OpFwdRef {
							t.Fatal("phi insertion incomplete")
						}
					}
				}
				text[j] = f.String()
			}
			native := backend.BuildSSA(functions[2], 3, false)
			if saved[0].Func().Cache == saved[1].Func().Cache || saved[0].Func().Cache == native.Cache {
				t.Fatal("construction reused worker storage")
			}
			for j := range saved {
				if saved[j].Func().String() != text[j] {
					t.Fatal("later build corrupted retained graph")
				}
			}
			first := saved[0].ContinueNative()
			if first != saved[0].Func() || first.RegAlloc == nil || functions[0].DebugInfo == nil || !saved[0].NativeCompleted() {
				t.Fatal("native continuation incomplete")
			}
			if saved[1].Func().String() != text[1] {
				t.Fatal("continuation corrupted another graph")
			}
			second := saved[1].ContinueNative()
			if second.RegAlloc == nil || !saved[1].NativeCompleted() {
				t.Fatal("second continuation incomplete")
			}
			defer func() {
				if recover() == nil {
					t.Error("continuation ran twice")
				}
			}()
			saved[0].ContinueNative()
		})
	}
}

func TestEarlyOptimizationAndNativeContinuation(t *testing.T) {
	const source = `package p
func F(p *int,n int)int {s:=n*0;for i:=range n{if i&1==0{s+=i}else{s-=i}};if p!=nil{s+=*p};return s}
func D(p *int)int {defer func(){*p+=3}();return *p}
`
	for _, seed := range []int{0, 1, 314159} {
		t.Run(fmt.Sprint(seed), func(t *testing.T) {
			t.Parallel()
			compile := func(early bool) []string {
				c, abis := loadQueueCompiler(t, 1, source, true)
				if seed != 0 {
					if err := c.Backend.SSA.PhaseOption("check", "seed", seed, ""); err != "" {
						t.Fatal(err)
					}
				}
				c.PrepareFrontend(abis, nil)
				var result []string
				for _, fn := range c.Package.Noder.Typecheck.Target.Funcs {
					c.EnqueueFunc(fn, abis)
					h := c.Backend.ConstructSSA(fn, false)
					f := h.Func()
					if early {
						copyHandle := *h
						if copyHandle.EarlyOptimize() != f || h.NativeCompleted() || f.RegAlloc != nil || len(f.RegArgs) != 0 {
							t.Fatal("early boundary crossed native tail")
						}
						mustPanic := func(run func()) {
							t.Helper()
							defer func() {
								if recover() == nil {
									t.Error("consumed continuation accepted")
								}
							}()
							run()
						}
						mustPanic(func() { h.EarlyOptimize() })
					}
					h.ContinueNative()
					if !h.NativeCompleted() || h.Func() != f || f.RegAlloc == nil {
						t.Fatal("native suffix incomplete")
					}
					result = append(result, f.String())
				}
				return result
			}
			native, early := compile(false), compile(true)
			if !slices.Equal(native, early) {
				t.Fatalf("resumed SSA differs for seed %d\nnative:%v\nearly:%v", seed, native, early)
			}
		})
	}
}

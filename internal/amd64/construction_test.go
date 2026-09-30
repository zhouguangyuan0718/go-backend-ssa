package amd64

import (
	"fmt"
	"go-backend-ssa/internal/ssa"
	"go-backend-ssa/internal/ssagen"
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

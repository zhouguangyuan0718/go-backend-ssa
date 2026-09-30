package ssagen

import (
	"bytes"
	"fmt"
	"go-backend-ssa/internal/ir"
	"go-backend-ssa/internal/liveness"
	"go-backend-ssa/internal/ssa"
	"go-backend-ssa/internal/types"
	"strings"
	"testing"
)

func frameContext(arch string, merge bool) *Context {
	c := frontendContext(arch, false)
	c.Liveness = liveness.NewContext(c.SSA)
	if merge {
		c.Typecheck.IR.Debug.MergeLocals = 1
	}
	c.InitConfig()
	return c
}
func frameGraph(c *Context, name string) (*ssa.Func, *ir.Func, *ssa.Value) {
	fn := frontendFunction(c, name)
	fn.DebugInfo = new(ssa.FuncDebug)
	cfg := c.SSAConfig()
	cache := new(ssa.Cache)
	cache.Reset()
	f := cfg.NewFunc(c.NewFrontend(fn, false), cache)
	f.Name = name
	f.Type = fn.Type()
	f.ABI0 = cfg.ABI0
	f.ABI1 = cfg.ABI1
	f.ABISelf = cfg.ABI1
	f.ABIDefault = cfg.ABI1
	f.OwnAux = ssa.OwnAuxCall(fn.Linksym(), cfg.ABI1.ABIAnalyze(fn.Type(), true))
	f.Entry = f.NewBlock(ssa.BlockRet)
	f.Entry.Pos = fn.Pos()
	return f, fn, f.Entry.NewValue0(fn.Pos(), ssa.OpInitMem, c.SSA.Types.TypeMem)
}
func finishFrameGraph(c *Context, f *ssa.Func, mem *ssa.Value) {
	f.Entry.SetControl(f.Entry.NewValue1(f.Entry.Pos, ssa.OpMakeResult, c.SSA.Types.TypeResultMem, mem))
	if err := c.SSA.PhaseOption("check", "on", 1, ""); err != "" {
		panic(err)
	}
	ssa.Compile(f)
}
func framePair(c *Context, overlap bool) (*ssa.Func, *ir.Func, *ir.Name, *ir.Name) {
	f, fn, mem := frameGraph(c, "pair")
	typ := c.SSA.Types.NewArray(c.SSA.Types.NewPtr(c.SSA.Types.Types[types.TUINT8]), 4)
	a := fn.NewLocal(fn.Pos(), c.SSA.Types.LocalPkg.Lookup("a"), typ)
	b := fn.NewLocal(fn.Pos(), c.SSA.Types.LocalPkg.Lookup("b"), typ)
	effect := func(op ssa.Op, n *ir.Name) { mem = f.Entry.NewValue1A(fn.Pos(), op, c.SSA.Types.TypeMem, n, mem) }
	effect(ssa.OpVarDef, a)
	if !overlap {
		effect(ssa.OpVarLive, a)
	}
	effect(ssa.OpVarDef, b)
	if overlap {
		effect(ssa.OpVarLive, a)
	}
	effect(ssa.OpVarLive, b)
	finishFrameGraph(c, f, mem)
	return f, fn, a, b
}
func TestNativeFrameMerging(t *testing.T) {
	for _, arch := range []string{"386", "amd64", "arm64", "ppc64", "riscv64", "wasm"} {
		for _, merge := range []bool{false, true} {
			for _, overlap := range []bool{false, true} {
				t.Run(fmt.Sprint(arch, "/", merge, "/", overlap), func(t *testing.T) {
					c := frameContext(arch, merge)
					f, fn, a, b := framePair(c, overlap)
					var trace bytes.Buffer
					c.Typecheck.IR.TraceWriter = &trace
					c.Typecheck.IR.Debug.MergeLocalsTrace = 4
					size, ptrsize, align := c.AllocFrame(f)
					want := int64(8 * c.SSA.Types.PointerSize())
					if merge && !overlap {
						want = int64(4 * c.SSA.Types.PointerSize())
					}
					if size != want || ptrsize != want || align != int64(c.SSA.Types.RegisterSize()) || ((a.FrameOffset() == b.FrameOffset()) != (merge && !overlap)) {
						t.Fatalf("size=%d ptr=%d align=%d a=%d b=%d\n%s", size, ptrsize, align, a.FrameOffset(), b.FrameOffset(), f)
					}
					if len(fn.Dcl) != 2 || a.Curfn != fn || b.Curfn != fn {
						t.Fatal("declaration identity")
					}
					if merge && !strings.Contains(trace.String(), "raw cand list") {
						t.Fatal("native diagnostics missing")
					}
					if !merge && trace.Len() != 0 {
						t.Fatal("disabled merge logged")
					}
				})
			}
		}
	}
}
func TestNativeFrameLayoutAndOffsets(t *testing.T) {
	for _, arch := range []string{"386", "amd64", "arm64", "ppc64", "riscv64", "wasm"} {
		t.Run(arch, func(t *testing.T) {
			c := frameContext(arch, false)
			f, fn, mem := frameGraph(c, "layout")
			u := c.SSA.Types
			local := func(name string, typ *types.Type, live bool) *ir.Name {
				n := fn.NewLocal(fn.Pos(), u.LocalPkg.Lookup(name), typ)
				if live {
					mem = f.Entry.NewValue1A(fn.Pos(), ssa.OpVarLive, u.TypeMem, n, mem)
				}
				return n
			}
			unused := local("unused", u.NewArray(u.Types[types.TUINT8], 17), false)
			ptr := local("ptr", u.NewPtr(u.Types[types.TUINT8]), true)
			ptr.SetNeedzero(true)
			zero := local("zero", u.NewArray(u.Types[types.TUINT8], 0), true)
			scalar := local("scalar", u.Types[types.TUINT8], true)
			defer1 := local("defer1", u.Types[types.TUINT8], false)
			defer1.SetOpenDeferSlot(true)
			defer1.SetUsed(true)
			defer1.SetFrameOffset(0)
			defer2 := local("defer2", u.Types[types.TUINT8], false)
			defer2.SetOpenDeferSlot(true)
			defer2.SetUsed(true)
			defer2.SetFrameOffset(1)
			finishFrameGraph(c, f, mem)
			size, ptrsize, align := c.AllocFrame(f)
			if ptr.FrameOffset() != -int64(u.PointerSize()) || ptrsize != int64(u.PointerSize()) || size%align != 0 || zero.FrameOffset() >= ptr.FrameOffset() || defer1.FrameOffset() >= defer2.FrameOffset() {
				t.Fatalf("layout %d/%d/%d ptr=%d zero=%d scalar=%d defer=%d/%d", size, ptrsize, align, ptr.FrameOffset(), zero.FrameOffset(), scalar.FrameOffset(), defer1.FrameOffset(), defer2.FrameOffset())
			}
			debug := fn.DebugInfo.(*ssa.FuncDebug)
			if len(debug.OptDcl) != 1 || debug.OptDcl[0] != unused || len(fn.Dcl) != 5 {
				t.Fatal("unused declaration retention")
			}
			for _, n := range fn.Dcl {
				slot := ssa.LocalSlot{N: n, Type: n.Type(), Off: 3}
				want := n.FrameOffset() + 3
				if c.Arch.LinkArch.FixedFrameSize == 0 {
					want -= int64(u.PointerSize())
				}
				if c.SSA.FramePointerEnabled {
					want -= int64(u.PointerSize())
				}
				if c.StackOffset(slot) != int32(want) {
					t.Fatal("stack offset")
				}
			}
			// Parameters keep caller-frame offsets independently of the local frame.
			for _, class := range []ir.Class{ir.PPARAM, ir.PPARAMOUT} {
				n := fn.NewLocal(fn.Pos(), u.LocalPkg.Lookup(fmt.Sprint(class)), u.Types[types.TINT])
				n.Class = class
				n.SetFrameOffset(24)
				if c.StackOffset(ssa.LocalSlot{N: n, Type: n.Type(), Off: 3}) != int32(27+c.Arch.LinkArch.FixedFrameSize) {
					t.Fatal("parameter offset")
				}
			}
		})
	}
}
func TestNativeFrameParallelIsolation(t *testing.T) {
	for i := range 8 {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			t.Parallel()
			c := frameContext("amd64", true)
			f, fn, a, b := framePair(c, false)
			other := frameContext("amd64", true)
			frontendPanic(t, "matching liveness", func() { other.AllocFrame(f) })
			if a.FrameOffset() != 0 || b.FrameOffset() != 0 {
				t.Fatal("foreign rejection changed layout")
			}
			size, _, _ := c.AllocFrame(f)
			if size != 32 || len(fn.Dcl) != 2 {
				t.Fatal("parallel frame")
			}
			if c.Liveness.SSA == other.Liveness.SSA {
				t.Fatal("shared services")
			}
		})
	}
}

func TestNativeFrameRejectsForeignVariables(t *testing.T) {
	for _, where := range []string{"declaration", "register home", "value auxiliary"} {
		t.Run(where, func(t *testing.T) {
			c := frameContext("amd64", true)
			f, fn, a, _ := framePair(c, false)
			other := frameContext("amd64", true)
			foreignFn := frontendFunction(other, "foreign")
			foreign := foreignFn.NewLocal(foreignFn.Pos(), other.SSA.Types.LocalPkg.Lookup("foreignVar"), other.SSA.Types.Types[types.TUINT64])
			foreign.SetUsed(true)
			beforeLocal, beforeForeign := a.Used(), foreign.Used()
			switch where {
			case "declaration":
				fn.Dcl = append(fn.Dcl, foreign)
			case "register home":
				f.RegAlloc = append(f.RegAlloc, ssa.LocalSlot{N: foreign, Type: foreign.Type()})
			case "value auxiliary":
				f.Entry.Values[0].Aux = foreign
			}
			frontendPanic(t, "frame variable belongs", func() { c.AllocFrame(f) })
			if a.Used() != beforeLocal || foreign.Used() != beforeForeign || a.FrameOffset() != 0 || foreign.FrameOffset() != 0 {
				t.Fatal("rejection changed frame state")
			}
		})
	}
}

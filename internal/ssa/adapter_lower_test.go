package ssa

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"go-backend-ssa/internal/abi"
	"go-backend-ssa/internal/cmd/obj"
	"go-backend-ssa/internal/cmd/src"
	"go-backend-ssa/internal/cmd/sys"
	"go-backend-ssa/internal/ir"
	"go-backend-ssa/internal/logopt"
	"go-backend-ssa/internal/objw"
	"go-backend-ssa/internal/reflectdata"
	"go-backend-ssa/internal/rttype"
	"go-backend-ssa/internal/staticdata"
	"go-backend-ssa/internal/typecheck"
	"go-backend-ssa/internal/types"
	"strings"
	"sync"
	"testing"
)

func loweringContext(arch string) *Context {
	size := 8
	var order binary.ByteOrder = binary.LittleEndian
	switch arch {
	case "386", "arm", "mips", "mipsle":
		size = 4
	}
	switch arch {
	case "mips", "mips64", "ppc64", "s390x":
		order = binary.BigEndian
	}
	link := &obj.Link{Arch: &obj.LinkArch{Arch: &sys.Arch{Name: arch, PtrSize: size, RegSize: size, ByteOrder: order, FixedFrameSize: 16}}}
	u := types.NewUniverseWithPositions(size, size, 1<<50, &link.PosTable)
	u.LocalPkg = u.NewPkg("example/p", "p")
	graph := ir.NewContext(u, link)
	tc := typecheck.NewContext(graph)
	tc.InitUniverse()
	rt := rttype.NewContext(objw.NewContext(u, link, "linux"))
	rt.Init()
	c := NewContext(abi.NewContext(graph), reflectdata.NewContext(tc, rt, staticdata.NewContext(graph, rt.Writer)), logopt.NewContext())
	c.GOAMD64 = 1
	c.GOARM.Version = 7
	c.GOPPC64 = 8
	c.GORISCV64 = 20
	return c
}
func loweringGraph(c *Context) *Func {
	cfg := c.NewConfig(c.IR.Base.Ctxt.Arch.Name, *c.NewTypes(), c.IR.Base.Ctxt, true, false)
	cache := new(Cache)
	cache.Reset()
	f := cfg.NewFunc(&deadcodeFrontend{diagnosticFrontend: &diagnosticFrontend{}}, cache)
	f.Name = "lower"
	f.Type = c.Types.NewSignature(nil, nil, nil)
	f.ABISelf = cfg.ABI1
	f.pass = &pass{name: "lower", debug: 2}
	b := f.NewBlock(BlockRet)
	f.Entry = b
	b.Pos = cfg.ctxt.PosTable.XPos(src.MakePos(src.NewFileBase("lower.go", "lower.go"), 10, 1))
	it := cfg.Types.Int32
	x := b.NewValue0(b.Pos, OpArg, it)
	y := b.NewValue0I(b.Pos, OpArg, it, 4)
	mem := b.NewValue0(b.Pos, OpInitMem, c.Types.TypeMem)
	sp := b.NewValue0(b.Pos, OpSP, cfg.Types.Uintptr)
	ptr := b.NewValue1I(b.Pos, OpOffPtr, c.Types.NewPtr(it), 16, sp)
	v := b.NewValue2(b.Pos, OpAdd32, it, x, y)
	v = b.NewValue2(b.Pos, OpMul32, it, v, f.ConstInt32(it, 9))
	mem = b.NewValue3A(b.Pos, OpStore, c.Types.TypeMem, it, ptr, v, mem)
	b.SetControl(mem)
	checkFunc(f)
	numberLines(f)
	lower(f)
	lateLower(f)
	deadcode(f)
	checkFunc(f)
	checkLower(f)
	return f
}
func TestNativeLoweringContextIsolation(t *testing.T) {
	old := loweringContext("386")
	oldTrace := new(bytes.Buffer)
	old.TraceWriter = oldTrace
	retained := loweringGraph(old)
	before := retained.String()
	trace := oldTrace.String()
	clobbers := old.opcodeTable[Op386LoweredWB].reg.clobbers
	var wg sync.WaitGroup
	for i := range 8 {
		wg.Go(func() {
			arch := []string{"amd64", "arm64", "386", "ppc64"}[i%4]
			c := loweringContext(arch)
			c.IR.Base.Ctxt.Flag_shared = true
			c.GOAMD64 = 1 + i%4
			c.GOPPC64 = 8 + i%3
			buf := new(bytes.Buffer)
			c.TraceWriter = buf
			f := loweringGraph(c)
			if !strings.Contains(buf.String(), "rewriting for lower") || f.Config.ABI1.Universe() != c.Types {
				t.Error("lowering lost compilation bindings")
			}
			if !c.opcodeTable[Op386LoweredWB].reg.clobbers.hasReg(3) {
				t.Error("native shared-library clobber missing")
			}
			if old.opcodeTable[Op386LoweredWB].reg.clobbers != clobbers {
				t.Error("opcode table leaked")
			}
		})
	}
	wg.Wait()
	if retained.String() != before || oldTrace.String() != trace {
		t.Fatal("retained graph or trace changed")
	}
}
func TestNativeLoweringRejectsForeignInputs(t *testing.T) {
	c, other := loweringContext("amd64"), loweringContext("amd64")
	reject := func(name string, fn func()) {
		t.Run(name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Error("input accepted")
				}
			}()
			fn()
		})
	}
	reject("ABI", func() { NewContext(other.ABI, c.ReflectData, c.Log) })
	reject("nil-log", func() { NewContext(c.ABI, c.ReflectData, nil) })
	reject("foreign-types", func() { c.NewConfig("amd64", *other.NewTypes(), c.IR.Base.Ctxt, true, false) })
	reject("foreign-link", func() { c.NewConfig("amd64", *c.NewTypes(), other.IR.Base.Ctxt, true, false) })
	reject("architecture", func() { c.NewConfig("386", *c.NewTypes(), c.IR.Base.Ctxt, true, false) })
	reject("pointer-type", func() {
		ts := *c.NewTypes()
		ts.BytePtr = other.NewTypes().BytePtr
		c.NewConfig("amd64", ts, c.IR.Base.Ctxt, true, false)
	})
	reject("vector-type", func() {
		ts := *c.NewTypes()
		ts.Vec512 = other.NewTypes().Vec512
		c.NewConfig("amd64", ts, c.IR.Base.Ctxt, true, false)
	})
	reject("feature-level", func() {
		copy := *c
		copy.GOAMD64 = 0
		copy.NewConfig("amd64", *copy.NewTypes(), copy.IR.Base.Ctxt, true, false)
	})
	reject("byte-order", func() {
		bad := loweringContext("amd64")
		bad.IR.Base.Ctxt.Arch.ByteOrder = binary.BigEndian
		bad.NewConfig("amd64", *bad.NewTypes(), bad.IR.Base.Ctxt, true, false)
	})
	reject("layout", func() {
		bad := loweringContext("amd64")
		bad.IR.Base.Ctxt.Arch.Name = "386"
		bad.NewConfig("386", *bad.NewTypes(), bad.IR.Base.Ctxt, true, false)
	})

	reject("changed-service", func() {
		copy := *c
		copy.ABI = other.ABI
		copy.NewConfig("amd64", *copy.NewTypes(), copy.IR.Base.Ctxt, true, false)
	})
	// Same type universe does not grant a different IR graph ownership.
	reject("same-universe-foreign-IR", func() { NewContext(abi.NewContext(ir.NewContext(c.Types, c.IR.Base.Ctxt)), c.ReflectData, c.Log) })
	f := loweringGraph(c)
	if f.Config.Ctxt() != c.IR.Base.Ctxt {
		t.Fatal("valid construction failed after rejection")
	}
}
func TestNativeAllArchitectureLowering(t *testing.T) {
	for _, arch := range []string{"386", "amd64", "arm", "arm64", "ppc64", "ppc64le", "mips", "mipsle", "mips64", "mips64le", "loong64", "riscv64", "s390x", "wasm"} {
		t.Run(arch, func(t *testing.T) {
			c := loweringContext(arch)
			f := loweringGraph(c)
			if f.Config.lowerValue == nil || f.Config.lowerBlock == nil {
				t.Fatal("missing native callback")
			}
			if f.Config.Types.Int.Size() != int64(c.Types.PointerSize()) {
				t.Fatal("layout mismatch")
			}
		})
	}
}
func TestNativeLoweringCheckRejectsUnlowered(t *testing.T) {
	c := loweringContext("amd64")
	f := loweringGraph(c)
	b := f.Entry
	b.NewValue2(b.Pos, OpAdd32, f.Config.Types.Int32, f.ConstInt32(f.Config.Types.Int32, 1), f.ConstInt32(f.Config.Types.Int32, 2))
	defer func() {
		if r := recover(); r == nil || !strings.Contains(fmt.Sprint(r), "not lowered") {
			t.Errorf("unexpected diagnostic %v", r)
		}
	}()
	checkLower(f)
}

func TestNativeLoweringFeatureCallbacksRetainOwner(t *testing.T) {
	probe := func(c *Context) Op {
		cfg := c.NewConfig("amd64", *c.NewTypes(), c.IR.Base.Ctxt, true, false)
		cache := new(Cache)
		cache.Reset()
		f := cfg.NewFunc(&diagnosticFrontend{}, cache)
		b := f.NewBlock(BlockRet)
		f.Entry = b
		x := b.NewValue0(src.XPos{}, OpArg, cfg.Types.Int32)
		y := b.NewValue0I(src.XPos{}, OpArg, cfg.Types.Int32, 4)
		n := b.NewValue1(src.XPos{}, OpAMD64NOTL, cfg.Types.Int32, y)
		v := b.NewValue2(src.XPos{}, OpAMD64ANDL, cfg.Types.Int32, x, n)
		cfg.lowerValue(v)
		return v.Op
	}
	low, high := loweringContext("amd64"), loweringContext("amd64")
	high.GOAMD64 = 4
	if got := probe(low); got != OpAMD64ANDL {
		t.Fatalf("v1 selected %s", got)
	}
	if got := probe(high); got != OpAMD64ANDNL {
		t.Fatalf("v4 selected %s", got)
	}
	if got := probe(low); got != OpAMD64ANDL {
		t.Fatalf("v4 changed v1 callback: %s", got)
	}
}

func TestNativeOtherFeatureCallbacksRetainOwner(t *testing.T) {
	for _, arch := range []string{"arm", "ppc64", "riscv64"} {
		t.Run(arch, func(t *testing.T) {
			low, high := loweringContext(arch), loweringContext(arch)
			low.GOARM.Version = 5
			high.GOARM.Version = 7
			low.GOPPC64 = 8
			high.GOPPC64 = 10
			low.GORISCV64 = 20
			high.GORISCV64 = 23
			probe := func(c *Context) Op {
				cfg := c.NewConfig(arch, *c.NewTypes(), c.IR.Base.Ctxt, true, false)
				cache := new(Cache)
				cache.Reset()
				f := cfg.NewFunc(&diagnosticFrontend{}, cache)
				b := f.NewBlock(BlockRet)
				f.Entry = b
				typ := cfg.Types.Int64
				if arch == "arm" {
					typ = cfg.Types.Float64
				}
				x := b.NewValue0(src.XPos{}, OpArg, typ)
				y := b.NewValue0I(src.XPos{}, OpArg, typ, 8)
				var v *Value
				switch arch {
				case "arm":
					z := b.NewValue2(src.XPos{}, OpARMMULD, typ, x, y)
					a := b.NewValue0I(src.XPos{}, OpArg, typ, 16)
					v = b.NewValue2(src.XPos{}, OpARMADDD, typ, a, z)
				case "ppc64":
					v = b.NewValue1(src.XPos{}, OpBswap64, typ, x)
				case "riscv64":
					v = b.NewValue2(src.XPos{}, OpMax64, typ, x, y)
				}
				cfg.lowerValue(v)
				return v.Op
			}
			before := probe(low)
			after := probe(high)
			again := probe(low)
			if before == after || before != again {
				t.Fatalf("feature callbacks: low=%s high=%s retained=%s", before, after, again)
			}
		})
	}
}

package ssa

import (
	"fmt"
	"go-backend-ssa/internal/cmd/obj"
	"go-backend-ssa/internal/cmd/sys"
	"go-backend-ssa/internal/types"
	"math/rand"
	"sync"
	"testing"
)

func allocationPipeline(f *Func) {
	passes := []pass{{name: "number lines", fn: numberLines}, {name: "opt", fn: opt}, {name: "early deadcode", fn: deadcode}, {name: "expand calls", fn: expandCalls}, {name: "decompose builtin", fn: postExpandCallsDecompose}, {name: "softfloat", fn: softfloat}, {name: "late opt", fn: opt}, {name: "generic deadcode", fn: deadcode}, {name: "writebarrier", fn: writebarrier}, {name: "lower", fn: lower}, {name: "addressing modes", fn: addressingModes}, {name: "late lower", fn: lateLower}, {name: "pair", fn: pair}, {name: "lowered deadcode for cse", fn: deadcode}, {name: "lowered cse", fn: cse}, {name: "elim unread autos", fn: elimUnreadAutos}, {name: "tighten tuple selectors", fn: tightenTupleSelectors}, {name: "lowered deadcode", fn: deadcode}, {name: "checkLower", fn: checkLower}, {name: "loop invariant", fn: licm}, {name: "late phielim and copyelim", fn: copyelim}, {name: "tighten", fn: tighten}, {name: "late deadcode", fn: deadcode}, {name: "critical", fn: critical}, {name: "phi tighten", fn: phiTighten}, {name: "likelyadjust", fn: likelyadjust}, {name: "layout", fn: layout}, {name: "schedule", fn: schedule}, {name: "late nilcheck", fn: nilcheckelim2}, {name: "flagalloc", fn: flagalloc}, {name: "regalloc", fn: regalloc}, {name: "loop rotate", fn: loopRotate}, {name: "trim", fn: trim}}
	checkFunc(f)
	for _, p := range passes {
		f.pass = &p
		p.fn(f)
		checkFunc(f)
	}
}

// Evaluate allocated instructions through physical homes, not SSA value IDs.
// All loads precede an aliasing store, forcing more simultaneously live values
// than available registers. Lost spills/reloads or reused live homes corrupt the
// result, even when the SSA use graph itself still describes the correct sum.
func TestNativeAllocatedPhysicalValues(t *testing.T) {
	for _, fp := range []bool{false, true} {
		for _, clobber := range []bool{false, true} {
			t.Run(fmt.Sprint(fp, "/", clobber), func(t *testing.T) {
				c := loweringContext("amd64")
				c.FramePointerEnabled = fp
				c.IR.Base.Flag.ClobberDeadReg = clobber
				f := allocationPressure(c)
				stores, reloads := 0, 0
				for _, b := range f.Blocks {
					for _, v := range b.Values {
						if v.Op == OpStoreReg {
							stores++
						}
						if v.Op == OpLoadReg {
							reloads++
						}
						if r, ok := f.getHome(v.ID).(*Register); ok && fp && r.num == int32(f.Config.FPReg) {
							t.Fatal("reserved frame-pointer register allocated")
						}
					}
				}
				if stores == 0 || reloads == 0 {
					t.Fatalf("pressure did not exercise spills: %d/%d", stores, reloads)
				}
				rng := rand.New(rand.NewSource(943))
				for sample := 0; sample < 200; sample++ {
					memory := map[uint64]uint64{}
					var sum, xor uint64
					for i := 0; i < 40; i++ {
						v := rng.Uint64()
						memory[0x1000+uint64(i*8)] = v
						sum += v
						xor ^= v
					}
					want := sum + xor
					homes := map[Location]uint64{}
					read := func(v *Value) uint64 {
						loc := f.getHome(v.ID)
						n, ok := homes[loc]
						if !ok {
							t.Fatalf("uninitialized physical home %v for %v", loc, v.LongString())
						}
						return n
					}
					var got uint64
					for _, b := range f.Blocks {
						for _, v := range b.Values {
							a := func(i int) uint64 { return read(v.Args[i]) }
							sym := uint64(0)
							if s, ok := v.Aux.(*obj.LSym); ok {
								if s.Name != "input" {
									t.Fatalf("unexpected symbol %s", s.Name)
								}
								sym = 0x1000
							}
							var n uint64
							write := true
							switch v.Op {
							case OpInitMem:
								write = false
							case OpSB:
								n = 0
							case OpSP:
								n = 0x100000
							case OpCopy, OpStoreReg, OpLoadReg:
								n = a(0)
							case OpClobberReg:
								delete(homes, f.getHome(v.ID))
								write = false
							case OpAMD64MOVQconst:
								n = uint64(v.AuxInt)
							case OpAMD64LEAQ:
								n = a(0) + sym + uint64(v.AuxInt)
							case OpAMD64MOVQload:
								n = memory[a(0)+sym+uint64(v.AuxInt)]
							case OpAMD64MOVQstore:
								memory[a(0)+sym+uint64(v.AuxInt)] = a(1)
								write = false
							case OpAMD64MOVQstoreconst:
								vo := ValAndOff(v.AuxInt)
								memory[a(0)+sym+uint64(int64(vo.Off()))] = uint64(int64(vo.Val()))
								write = false
							case OpAMD64ADDQ:
								n = a(0) + a(1)
							case OpAMD64XORQ:
								n = a(0) ^ a(1)
							case OpAMD64ADDQload:
								n = a(0) + memory[a(1)+sym+uint64(v.AuxInt)]
							case OpAMD64XORQload:
								n = a(0) ^ memory[a(1)+sym+uint64(v.AuxInt)]
							case OpMakeResult:
								got = a(0)
								write = false
							default:
								t.Fatalf("unsupported allocated instruction %v", v.LongString())
							}
							if write {
								loc := f.getHome(v.ID)
								if loc == nil {
									t.Fatalf("result has no home: %v", v)
								}
								homes[loc] = n
							}
						}
					}
					if got != want || memory[0x1000] != 85 {
						t.Fatalf("sample %d physical result %x want %x memory %x", sample, got, want, memory[0x1000])
					}
				}
			})
		}
	}
}

func allocationPressure(c *Context) *Func {
	typ := c.Types.Types[types.TUINT64]
	f, _ := callsFuncType(c, c.Types.NewSignature(nil, nil, []*types.Field{c.Types.NewField(c.Types.AutogeneratedPos, nil, typ)}), false)
	b := f.Entry
	pos := b.Pos
	mem := b.NewValue0(pos, OpInitMem, c.Types.TypeMem)
	_, sb := f.spSb()
	ptr := b.NewValue1A(pos, OpAddr, c.Types.NewPtr(typ), c.IR.Base.Ctxt.Lookup("input"), sb)
	var loaded []*Value
	for i := 0; i < 40; i++ {
		p := b.NewValue1I(pos, OpOffPtr, ptr.Type, int64(i*8), ptr)
		loaded = append(loaded, b.NewValue2(pos, OpLoad, typ, p, mem))
	}
	mem = b.NewValue3A(pos, OpStore, c.Types.TypeMem, typ, ptr, f.ConstInt64(typ, 85), mem)
	sum, xor := f.ConstInt64(typ, 0), f.ConstInt64(typ, 0)
	for _, v := range loaded {
		sum = b.NewValue2(pos, OpAdd64, typ, sum, v)
		xor = b.NewValue2(pos, OpXor64, typ, xor, v)
	}
	result := b.NewValue2(pos, OpAdd64, typ, sum, xor)
	b.SetControl(b.NewValue2(pos, OpMakeResult, c.Types.NewResults([]*types.Type{typ, c.Types.TypeMem}), result, mem))
	allocationPipeline(f)
	return f
}

func allocationContext(arch string) *Context {
	old := loweringContext(arch)
	archs := sys.NewContext()
	for _, a := range archs.Archs {
		if a.Name == arch {
			old.IR.Base.Ctxt.Arch.Arch = a
		}
	}
	c := NewContext(old.ABI, old.ReflectData, old.Log)
	c.GOAMD64 = old.GOAMD64
	c.GOARM = old.GOARM
	c.GOPPC64 = old.GOPPC64
	c.GORISCV64 = old.GORISCV64
	c.FramePointerEnabled = arch == "amd64" || arch == "arm64"
	return c
}
func TestNativeAllocationCanonicalIdentity(t *testing.T) {
	c := allocationContext("riscv64")
	f, _ := callsFunc(c, false)
	s := regAllocState{f: f}
	mask := regMaskAt(2).addReg(10)
	if s.pickReg(c, mask) != 10 {
		t.Fatal("canonical RISC-V compressed-register preference lost")
	}
	copy := *c.IR.Base.Ctxt.Arch.Arch
	c.IR.Base.Ctxt.Arch.Arch = &copy
	if s.pickReg(c, mask) != 2 {
		t.Fatal("copied descriptor acquired canonical identity")
	}
	other := allocationContext("riscv64")
	f2, _ := callsFunc(other, false)
	s2 := regAllocState{f: f2}
	if s2.pickReg(other, mask) != 10 || c.archRISCV64 == other.archRISCV64 {
		t.Fatal("architecture identity shared across compilers")
	}
}
func TestNativeAllocationContextIsolation(t *testing.T) {
	old := allocationContext("386")
	retained := allocationPressure(old)
	before := retained.String()
	var wg sync.WaitGroup
	for i := range 8 {
		wg.Go(func() {
			c := allocationContext([]string{"amd64", "arm64", "riscv64", "wasm"}[i%4])
			c.IR.Base.Flag.ClobberDeadReg = i%2 == 0
			f := allocationPressure(c)
			if !f.scheduled || f.RegAlloc == nil || f.Cache.stackAllocState == nil || f.Cache.stackAllocState.f != nil {
				t.Error("allocation/cache lifecycle")
			}
			for _, slot := range f.RegAlloc {
				if s, ok := slot.(LocalSlot); ok && s.Type.Universe() != c.Types {
					t.Error("foreign stack slot")
				}
			}
			if c.noRegisters[0] != noRegister || c.tmpVal.ID != 0 {
				t.Error("sentinel leaked")
			}
			c.noRegisters[0] = 0
			c.tmpVal.ID = 27
			delete(c.combineFirst, OpAMD64MOVQload)
			delete(c.pairableLoads, OpARM64MOVDload)
		})
	}
	wg.Wait()
	if retained.String() != before || old.noRegisters[0] != noRegister || old.tmpVal.ID != 0 {
		t.Fatal("another allocation changed a retained compilation")
	}
}

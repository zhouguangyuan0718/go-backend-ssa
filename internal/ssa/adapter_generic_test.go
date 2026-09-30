package ssa

import (
	"bytes"
	"fmt"
	"go-backend-ssa/internal/cmd/obj"
	"go-backend-ssa/internal/cmd/src"
	"go-backend-ssa/internal/cmd/sys"
	"go-backend-ssa/internal/ir"
	"go-backend-ssa/internal/types"
	"strings"
	"sync"
	"testing"
)

type genericFrontend struct {
	*diagnosticFrontend
	splits []string
}

func (fe *genericFrontend) Warnl(src.XPos, string, ...any) {}
func (fe *genericFrontend) SplitSlot(parent *LocalSlot, suffix string, offset int64, t *types.Type) LocalSlot {
	if parent.N.Class != ir.PPARAM {
		panic("fixture expects parameter slot")
	}
	fe.splits = append(fe.splits, suffix)
	return LocalSlot{N: parent.N, Type: t, Off: parent.Off + offset}
}
func genericFunc(c *Context) (*Func, *genericFrontend) {
	cfg := c.NewConfig(c.IR.Base.Ctxt.Arch.Name, *c.NewTypes(), c.IR.Base.Ctxt, true, false)
	cache := new(Cache)
	cache.Reset()
	fe := &genericFrontend{diagnosticFrontend: &diagnosticFrontend{}}
	f := cfg.NewFunc(fe, cache)
	f.Name = "generic"
	f.Type = c.Types.NewSignature(nil, nil, nil)
	f.ABISelf = cfg.ABI1
	f.pass = &pass{name: "generic", debug: 2}
	f.Entry = f.NewBlock(BlockRet)
	f.Entry.Pos = cfg.ctxt.PosTable.XPos(src.MakePos(src.NewFileBase("generic.go", "generic.go"), 10, 1))
	return f, fe
}
func genericPipeline(f *Func) {
	checkFunc(f)
	numberLines(f)
	decomposeUser(f)
	opt(f)
	divisible(f)
	divmod(f)
	decomposeBuiltin(f)
	softfloat(f)
	opt(f)
	deadcode(f)
	f.invalidateCFG()
	checkFunc(f)
}
func genericStringPhi(c *Context) *Func {
	f, _ := genericFunc(c)
	ty := f.Config.Types
	e := f.Entry
	pos := e.Pos
	e.Reset(BlockIf)
	e.SetControl(e.NewValue0(pos, OpArg, ty.Bool))
	l, r, j := f.NewBlock(BlockPlain), f.NewBlock(BlockPlain), f.NewBlock(BlockRet)
	l.Pos = pos
	r.Pos = pos
	j.Pos = pos
	e.AddEdgeTo(l)
	e.AddEdgeTo(r)
	l.AddEdgeTo(j)
	r.AddEdgeTo(j)
	mem := e.NewValue0(pos, OpInitMem, c.Types.TypeMem)
	data := e.NewValue0I(pos, OpArg, ty.BytePtr, 8)
	ptr := e.NewValue0I(pos, OpArg, c.Types.NewPtr(ty.String), 16)
	ci := func(k int64) *Value {
		if f.Config.PtrSize == 4 {
			return f.ConstInt32(ty.Int, int32(k))
		}
		return f.ConstInt64(ty.Int, k)
	}
	a := l.NewValue2(pos, OpStringMake, ty.String, data, ci(3))
	otherData := e.NewValue0I(pos, OpArg, ty.BytePtr, 24)
	b := r.NewValue2(pos, OpStringMake, ty.String, otherData, ci(7))
	phi := j.NewValue2(pos, OpPhi, ty.String, a, b)
	n := c.IR.NewNameAt(pos, c.Types.LocalPkg.Lookup("s"), ty.String)
	n.Class = ir.PPARAM
	slot := &LocalSlot{N: n, Type: ty.String}
	f.Names = []*LocalSlot{slot}
	f.NamedValues[*slot] = []*Value{phi, phi}
	mem = j.NewValue3A(pos, OpStore, c.Types.TypeMem, ty.String, ptr, phi, mem)
	j.SetControl(mem)
	genericPipeline(f)
	return f
}
func TestNativeGenericDecompositionIsolation(t *testing.T) {
	old := loweringContext("386")
	retained := genericStringPhi(old)
	before := retained.String()
	check := func(c *Context, f *Func) {
		if len(f.Names) != 2 {
			t.Errorf("string names: %v", f.Names)
			return
		}
		seen := map[int64]bool{}
		for _, slot := range f.Names {
			seen[slot.Off] = true
			if slot.Type.Universe() != c.Types || len(f.NamedValues[*slot]) == 0 {
				t.Error("split slot lost ownership or values")
			}
		}
		if !seen[0] || !seen[int64(c.Types.PointerSize())] {
			t.Errorf("split offsets: %v", seen)
		}
		lower(f)
		lateLower(f)
		deadcode(f)
		f.invalidateCFG()
		checkFunc(f)
		checkLower(f)
	}
	var wg sync.WaitGroup
	for i := range 8 {
		wg.Go(func() {
			arch := []string{"amd64", "mips", "ppc64", "arm64"}[i%4]
			c := loweringContext(arch)
			buf := new(bytes.Buffer)
			c.TraceWriter = buf
			f := genericStringPhi(c)
			check(c, f)
			if !strings.Contains(buf.String(), "rewriting for generic") {
				t.Error("lost trace binding")
			}
		})
	}
	wg.Wait()
	if retained.String() != before {
		t.Fatal("another compilation changed retained graph")
	}
	check(old, retained)
}
func TestNativeLocalSplitCacheAndFailure(t *testing.T) {
	c := loweringContext("amd64")
	f, fe := genericFunc(c)
	ty := f.Config.Types
	n := c.IR.NewNameAt(f.Entry.Pos, c.Types.LocalPkg.Lookup("s"), ty.String)
	n.Class = ir.PPARAM
	slot := &LocalSlot{N: n, Type: ty.String}
	p, l := f.SplitString(slot)
	p2, l2 := f.SplitString(slot)
	if p != p2 || l != l2 || len(fe.splits) != 2 || p.Type != ty.BytePtr || l.Type != ty.Int || l.Off != 8 {
		t.Fatal("native split cache or type selection")
	}
	at := c.Types.NewArray(ty.Int, 2)
	bad := c.IR.NewNameAt(f.Entry.Pos, c.Types.LocalPkg.Lookup("a"), at)
	bad.Class = ir.PPARAM
	count := len(f.CanonicalLocalSplits)
	func() {
		defer func() {
			if r := recover(); r == nil || !strings.Contains(fmt.Sprint(r), "bad array size") {
				t.Errorf("array rejection: %v", r)
			}
		}()
		f.SplitArray(&LocalSlot{N: bad, Type: at})
	}()
	if len(f.CanonicalLocalSplits) != count {
		t.Fatal("failed split changed cache")
	}
	if p3, l3 := f.SplitString(slot); p3 != p || l3 != l {
		t.Fatal("failed split changed existing entries")
	}
}
func genericEmptyAllocation(c *Context) (*Func, *obj.LSym) {
	f, _ := genericFunc(c)
	b := f.Entry
	ty := f.Config.Types
	pos := b.Pos
	symbol := &obj.LSym{Name: "runtime.zerobase"}
	c.IR.Syms.Zerobase = symbol
	info := f.Config.ABI1.ABIAnalyzeTypes([]*types.Type{ty.BytePtr, ty.Int, ty.Int}, []*types.Type{ty.BytePtr})
	aux := StaticAuxCall(&obj.LSym{Name: "runtime.makeslice"}, info)
	var zero *Value
	if f.Config.PtrSize == 4 {
		zero = f.ConstInt32(ty.Int, 0)
	} else {
		zero = f.ConstInt64(ty.Int, 0)
	}
	mem := b.NewValue0(pos, OpInitMem, c.Types.TypeMem)
	call := b.NewValue0A(pos, OpStaticLECall, aux.LateExpansionResultType(), aux)
	call.AddArgs(f.ConstNil(ty.BytePtr), zero, zero, mem)
	ptr := b.NewValue1I(pos, OpSelectN, ty.BytePtr, 0, call)
	next := b.NewValue1I(pos, OpSelectN, c.Types.TypeMem, 1, call)
	result := b.NewValue2(pos, OpMakeResult, c.Types.TypeMem, ptr, next)
	b.SetControl(result)
	genericPipeline(f)
	return f, symbol
}
func TestNativeGenericIRSymbolBinding(t *testing.T) {
	old, sym := genericEmptyAllocation(loweringContext("386"))
	before := old.String()
	for range 8 {
		f, s := genericEmptyAllocation(loweringContext("amd64"))
		found := false
		for _, b := range f.Blocks {
			for _, v := range b.Values {
				if v.Op == OpAddr {
					found = true
					if v.Aux != s || v.Aux == sym {
						t.Fatal("runtime symbol leaked")
					}
				}
			}
		}
		if !found {
			t.Fatal("empty allocation rule did not fire")
		}
	}
	if old.String() != before {
		t.Fatal("retained allocation graph changed")
	}
}
func TestNativeVolatilePointerClassification(t *testing.T) {
	c := loweringContext("amd64")
	f, _ := genericFunc(c)
	b := f.Entry
	for _, base := range []Op{OpSP, OpSB, OpArg} {
		v := b.NewValue0(b.Pos, base, f.Config.Types.Uintptr)
		for _, wrapper := range []Op{OpCopy, OpOffPtr, OpAddPtr, OpPtrIndex, OpSelectNAddr} {
			v = b.NewValue1(b.Pos, wrapper, v.Type, v)
			if isVolatile(v) != (base == OpSP) {
				t.Fatalf("%s through %s", base, wrapper)
			}
		}
	}
}
func TestNativeTernaryTruthTableAndFeatureGate(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		for _, arch := range []string{"amd64", "arm64"} {
			c := loweringContext(arch)
			c.IR.Base.Ctxt.Arch.Family = sys.AMD64
			if arch == "arm64" {
				c.IR.Base.Ctxt.Arch.Family = sys.ARM64
			}
			f, _ := genericFunc(c)
			b := f.Entry
			tvec := c.Types.TypeVec128
			if enabled {
				f.maxCPUFeatures = CPUavx512
				b.CPUfeatures = CPUavx512
			}
			x := b.NewValue0(b.Pos, OpArg, tvec)
			y := b.NewValue0I(b.Pos, OpArg, tvec, 16)
			z := b.NewValue0I(b.Pos, OpArg, tvec, 32)
			a := b.NewValue2(b.Pos, OpOrInt8x16, tvec, x, y)
			v := b.NewValue2(b.Pos, OpAndInt8x16, tvec, a, z)
			mem := b.NewValue0(b.Pos, OpInitMem, c.Types.TypeMem)
			ptr := b.NewValue0I(b.Pos, OpArg, c.Types.NewPtr(tvec), 48)
			store := b.NewValue3A(b.Pos, OpStore, c.Types.TypeMem, tvec, ptr, v, mem)
			b.SetControl(store)
			checkFunc(f)
			numberLines(f)
			rewriteTern(f)
			deadcode(f)
			checkFunc(f)
			root := store.Args[1]
			if (root.Op == OpternInt32x4) != (enabled && arch == "amd64") {
				t.Fatalf("%s enabled=%v selected %s", arch, enabled, root.Op)
			}
			for input := range 8 {
				values := map[*Value]uint8{x: uint8(input>>2) & 1, y: uint8(input>>1) & 1, z: uint8(input) & 1}
				var eval func(*Value) uint8
				eval = func(v *Value) uint8 {
					if n, ok := values[v]; ok {
						return n
					}
					switch v.Op {
					case OpCopy:
						return eval(v.Args[0])
					case OpOrInt8x16:
						return eval(v.Args[0]) | eval(v.Args[1])
					case OpAndInt8x16:
						return eval(v.Args[0]) & eval(v.Args[1])
					case OpternInt32x4:
						i := 4*eval(v.Args[0]) + 2*eval(v.Args[1]) + eval(v.Args[2])
						return uint8(uint64(v.AuxInt)>>i) & 1
					}
					t.Fatalf("unexpected logical op %s", v.Op)
					return 0
				}
				want := (values[x] | values[y]) & values[z]
				if got := eval(root); got != want {
					t.Fatalf("input %d got %d want %d", input, got, want)
				}
			}
		}
	}
}

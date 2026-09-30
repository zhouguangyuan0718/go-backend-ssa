package ssa

import (
	"fmt"
	"sort"
	"testing"
)

// Interpret the generic graph's memory effects on both sides of the barrier
// flag. Check the actual stores separately from the old/new pointer log.
func TestNativeWriteBarrierMemoryOrder(t *testing.T) {
	for _, arch := range []string{"386", "amd64", "mips", "ppc64", "arm64"} {
		t.Run(arch, func(t *testing.T) {
			c := loweringContext(arch)
			f, fe := callsFunc(c, false)
			b := f.Entry
			pos := b.Pos
			ty := f.Config.Types
			size := f.Config.PtrSize
			mem := b.NewValue0(pos, OpInitMem, c.Types.TypeMem)
			ptr := b.NewValue0(pos, OpArg, c.Types.NewPtr(ty.BytePtr))
			val := b.NewValue0I(pos, OpArg, ty.BytePtr, size)
			for i := 0; i < 10; i++ {
				dst := b.NewValue1I(pos, OpOffPtr, ptr.Type, int64(i)*size, ptr)
				mem = b.NewValue3A(pos, OpStore, c.Types.TypeMem, ty.BytePtr, dst, val, mem)
			}
			b.SetControl(mem)
			checkFunc(f)
			numberLines(f)
			writebarrier(f)
			checkFunc(f)
			if !fe.fn.WBPos.IsKnown() {
				t.Fatal("write-barrier position missing")
			}
			for _, enabled := range []bool{false, true} {
				heap := map[uint64]uint64{}
				for i := 0; i < 10; i++ {
					heap[0x1000+uint64(i)*uint64(size)] = uint64(0x2000 + i*8)
				}
				values := map[*Value]uint64{ptr: 0x1000, val: 0x9870}
				var chunks []int
				var buffered []uint64
				nextBuffer := uint64(0x100000)
				buffers := map[uint64]bool{}
				pred := -1
				var eval func(*Value) uint64
				eval = func(v *Value) uint64 {
					if n, ok := values[v]; ok {
						return n
					}
					var n uint64
					arg := func(i int) uint64 { return eval(v.Args[i]) }
					switch v.Op {
					case OpInitMem, OpSP, OpSB:
						n = 0
					case OpCopy, OpWBend:
						n = arg(0)
					case OpConst32, OpConst64:
						n = uint64(v.AuxInt)
					case OpAddr:
						n = 0x80000
					case OpOffPtr:
						n = arg(0) + uint64(v.AuxInt)
					case OpLoad:
						arg(1)
						a := arg(0)
						if a == 0x80000 {
							if enabled {
								n = 1
							}
						} else {
							n = heap[a]
						}
					case OpNeq32:
						if arg(0) != arg(1) {
							n = 1
						}
					case OpWB:
						arg(0)
						n = nextBuffer
						nextBuffer += 0x100
						chunks = append(chunks, int(v.AuxInt))
						for i := int64(0); i < v.AuxInt; i++ {
							buffers[n+uint64(i*size)] = true
						}
					case OpSelect0:
						n = arg(0)
					case OpSelect1:
						arg(0)
					case OpStore:
						arg(2)
						a, x := arg(0), arg(1)
						if buffers[a] {
							buffered = append(buffered, x)
						} else {
							heap[a] = x
						}
					case OpPhi:
						if pred < 0 {
							t.Fatal("phi without predecessor")
						}
						n = arg(pred)
					default:
						t.Fatalf("unhandled barrier oracle op %s", v.LongString())
					}
					values[v] = n
					return n
				}
				block := f.Entry
				for steps := 0; ; steps++ {
					if steps > 10 {
						t.Fatal("unexpected barrier cycle")
					}
					if block.Kind == BlockRet {
						eval(block.Controls[0])
						break
					}
					edge := 0
					if block.Kind == BlockIf && eval(block.Controls[0]) == 0 {
						edge = 1
					}
					next := block.Succs[edge]
					pred = next.i
					block = next.b
				}
				for i := 0; i < 10; i++ {
					if heap[0x1000+uint64(i)*uint64(size)] != 0x9870 {
						t.Fatalf("flag=%v store %d lost", enabled, i)
					}
				}
				if !enabled {
					if len(chunks) != 0 || len(buffered) != 0 {
						t.Fatal("disabled barrier wrote buffer")
					}
					continue
				}
				if fmt.Sprint(chunks) != "[8 3]" {
					t.Fatalf("buffer chunks %v", chunks)
				}
				want := []uint64{0x9870}
				for i := 0; i < 10; i++ {
					want = append(want, uint64(0x2000+i*8))
				}
				sort.Slice(want, func(i, j int) bool { return want[i] < want[j] })
				sort.Slice(buffered, func(i, j int) bool { return buffered[i] < buffered[j] })
				if fmt.Sprint(buffered) != fmt.Sprint(want) {
					t.Fatalf("old/new pointers got %x want %x", buffered, want)
				}
			}
			lower(f)
			lateLower(f)
			deadcode(f)
			f.invalidateCFG()
			checkFunc(f)
			checkLower(f)
		})
	}
}

func TestNativeWriteBarrierZeroRegions(t *testing.T) {
	c := loweringContext("amd64")
	f, _ := callsFunc(c, false)
	b := f.Entry
	pos := b.Pos
	ty := f.Config.Types
	mem := b.NewValue0(pos, OpInitMem, c.Types.TypeMem)
	ptr := b.NewValue0(pos, OpArg, c.Types.NewPtr(ty.BytePtr))
	value := b.NewValue0(pos, OpArg, ty.BytePtr)
	// An independently specified known-zero interval lets us check exact range,
	// alignment, memory-version, and heap-source boundaries without a GC runtime.
	zeroes := map[ID]ZeroRegion{mem.ID: {base: ptr, mask: 3}}
	mk := func(off int64) *Value {
		p := b.NewValue1I(pos, OpOffPtr, ptr.Type, off, ptr)
		return b.NewValue2IA(pos, OpZero, c.Types.TypeMem, 8, ty.BytePtr, p, mem)
	}
	for _, tt := range []struct {
		off  int64
		need bool
	}{{0, false}, {8, false}, {16, true}, {-8, true}, {1, true}, {512, true}} {
		v := mk(tt.off)
		if got := needwb(v, zeroes); got != tt.need {
			t.Fatalf("offset %d need %v want %v", tt.off, got, tt.need)
		}
	}
	store := b.NewValue3A(pos, OpStore, c.Types.TypeMem, ty.BytePtr, ptr, value, mem)
	if !needwb(store, zeroes) {
		t.Fatal("new heap pointer still needs barrier")
	}
	_, sb := f.spSb()
	global := b.NewValue1A(pos, OpAddr, ty.BytePtr, c.IR.Base.Ctxt.Lookup("global"), sb)
	store.SetArg(1, global)
	if needwb(store, zeroes) {
		t.Fatal("global pointer into zero memory needs no barrier")
	}
	fresh := b.NewValue0(pos, OpInitMem, c.Types.TypeMem)
	store.SetArg(2, fresh)
	if !needwb(store, zeroes) {
		t.Fatal("zero facts escaped their memory version")
	}
}

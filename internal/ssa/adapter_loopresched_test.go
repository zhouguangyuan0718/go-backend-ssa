package ssa

import (
	"fmt"
	"testing"
)

func TestNativeLoopRescheduleMemory(t *testing.T) {
	for _, arch := range []string{"386", "amd64", "mips", "ppc64", "arm64"} {
		for _, nosplit := range []bool{false, true} {
			t.Run(fmt.Sprint(arch, "/", nosplit), func(t *testing.T) {
				c := loweringContext(arch)
				f, _ := callsFunc(c, false)
				f.NoSplit = nosplit
				e := f.Entry
				pos := e.Pos
				ty := f.Config.Types
				n := e.NewValue0(pos, OpArg, ty.Int32)
				ptr := e.NewValue0I(pos, OpArg, c.Types.NewPtr(ty.Int32), 8)
				mem := e.NewValue0(pos, OpInitMem, c.Types.TypeMem)
				h, body, end := f.NewBlock(BlockIf), f.NewBlock(BlockPlain), f.NewBlock(BlockRet)
				for _, b := range []*Block{h, body, end} {
					b.Pos = pos
				}
				e.Kind = BlockPlain
				e.AddEdgeTo(h)
				h.AddEdgeTo(body)
				h.AddEdgeTo(end)
				body.AddEdgeTo(h)
				index := h.NewValue1(pos, OpPhi, ty.Int32, f.ConstInt32(ty.Int32, 0))
				index.AddArg(body.NewValue2(pos, OpAdd32, ty.Int32, index, f.ConstInt32(ty.Int32, 1)))
				memory := h.NewValue1(pos, OpPhi, c.Types.TypeMem, mem)
				memory.AddArg(body.NewValue3A(pos, OpStore, c.Types.TypeMem, ty.Int32, ptr, index, memory))
				end.SetControl(memory)
				h.SetControl(h.NewValue2(pos, OpLess32, ty.Bool, index, n))
				checkFunc(f)
				numberLines(f)
				insertLoopReschedChecks(f)
				checkFunc(f)
				for _, count := range []uint64{0, 1, 2, 37} {
					for _, slow := range []bool{false, true} {
						values := map[*Value]uint64{n: count, ptr: 0x1000}
						var stores []uint64
						calls := 0
						var eval func(*Value) uint64
						eval = func(v *Value) uint64 {
							if x, ok := values[v]; ok {
								return x
							}
							arg := func(i int) uint64 { return eval(v.Args[i]) }
							var x uint64
							switch v.Op {
							case OpInitMem:
								x = 0
							case OpConst32, OpConst64:
								x = uint64(v.AuxInt)
							case OpCopy:
								x = arg(0)
							case OpAdd32:
								x = uint64(uint32(arg(0) + arg(1)))
							case OpLess32:
								if int32(arg(0)) < int32(arg(1)) {
									x = 1
								}
							case OpLess32U, OpLess64U:
								if arg(0) < arg(1) {
									x = 1
								}
							case OpStore:
								arg(2)
								if arg(0) != 0x1000 {
									t.Fatal("store address changed")
								}
								stores = append(stores, arg(1))
								x = uint64(len(stores))
							case OpGetG:
								arg(0)
								x = 0x8000
							case OpSP:
								x = 1
							case OpOffPtr:
								x = arg(0) + uint64(v.AuxInt)
							case OpLoad:
								arg(1)
								if arg(0) != 0x8000+uint64(2*f.Config.PtrSize) {
									t.Fatal("wrong stack guard address")
								}
								if slow {
									x = 2
								}
							case OpStaticCall:
								arg(0)
								if v.Aux.(*AuxCall).Fn.Name != "runtime.goschedguarded" {
									t.Fatal("wrong scheduling call")
								}
								calls++
							case OpSelectN:
								x = arg(0)
							default:
								t.Fatalf("unsupported loop oracle op %s", v.LongString())
							}
							values[v] = x
							return x
						}
						b, pred := f.Entry, -1
						for steps := 0; ; steps++ {
							if steps > int(count)*5+10 {
								t.Fatal("loop failed to terminate")
							}
							phis := map[*Value]uint64{}
							for _, v := range b.Values {
								if v.Op == OpPhi {
									phis[v] = eval(v.Args[pred])
								}
							}
							for _, v := range b.Values {
								if v != n && v != ptr {
									delete(values, v)
								}
							}
							for v, x := range phis {
								values[v] = x
							}
							if b.Kind == BlockRet {
								eval(b.Controls[0])
								break
							}
							edge := 0
							if b.Kind == BlockIf && eval(b.Controls[0]) == 0 {
								edge = 1
							}
							next := b.Succs[edge]
							b, pred = next.b, next.i
						}
						if len(stores) != int(count) {
							t.Fatalf("count %d slow %v stored %v", count, slow, stores)
						}
						for i, v := range stores {
							if v != uint64(i) {
								t.Fatalf("store %d got %d", i, v)
							}
						}
						want := 0
						if slow && !nosplit {
							want = int(count)
						}
						if calls != want {
							t.Fatalf("scheduling calls %d want %d", calls, want)
						}
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
}

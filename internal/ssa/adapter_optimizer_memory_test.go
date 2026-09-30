package ssa

import (
	"bytes"
	"fmt"
	"go-backend-ssa/internal/types"
	"testing"
)

func TestNativeOptimizerCombinedStoreBytes(t *testing.T) {
	for _, arch := range []string{"386", "amd64", "arm64", "mips", "ppc64"} {
		t.Run(arch, func(t *testing.T) {
			c := loweringContext(arch)
			f, _ := genericFunc(c)
			ty := f.Config.Types
			b := f.Entry
			pos := b.Pos
			x := b.NewValue0(pos, OpArg, ty.UInt32)
			ptr := b.NewValue0I(pos, OpArg, ty.BytePtr, 16)
			mem := b.NewValue0(pos, OpInitMem, c.Types.TypeMem)
			for i := 0; i < 4; i++ {
				value := b.NewValue2(pos, OpRsh32Ux32, ty.UInt32, x, f.ConstInt32(ty.UInt32, int32(8*i)))
				value = b.NewValue1(pos, OpTrunc32to8, ty.UInt8, value)
				address := b.NewValue1I(pos, OpOffPtr, ty.BytePtr, int64(i), ptr)
				mem = b.NewValue3A(pos, OpStore, c.Types.TypeMem, ty.UInt8, address, value, mem)
			}
			b.SetControl(mem)
			checkFunc(f)
			numberLines(f)
			opt(f)
			deadcode(f)
			memcombine(f)
			deadcode(f)
			checkFunc(f)
			var stores []*Value
			for m := b.Controls[0]; m.Op != OpInitMem; {
				if m.Op == OpCopy {
					m = m.Args[0]
					continue
				}
				if m.Op != OpStore {
					t.Fatal("unexpected memory chain")
				}
				stores = append(stores, m)
				m = m.MemoryArg()
			}
			if f.Config.unalignedOK && len(stores) != 1 {
				t.Fatalf("stores did not combine: %s", f)
			}
			seed := uint32(12345)
			for i := 0; i < 1000; i++ {
				seed ^= seed << 13
				seed ^= seed >> 17
				seed ^= seed << 5
				out := make([]byte, 4)
				written := make([]bool, 4)
				for _, s := range stores {
					off := int64(0)
					p := s.Args[0]
					for p.Op == OpOffPtr {
						off += p.AuxInt
						p = p.Args[0]
					}
					if p != ptr {
						t.Fatal("foreign destination")
					}
					size := int(s.Aux.(*types.Type).Size())
					v := evalGenericInteger(t, s.Args[1], x, uint64(seed))
					for j := 0; j < size; j++ {
						index := int(off) + j
						if index < 0 || index >= len(out) || written[index] {
							t.Fatal("bad or overlapping combined store")
						}
						shift := j
						if f.Config.BigEndian {
							shift = size - 1 - j
						}
						out[index] = byte(v >> uint(shift*8))
						written[index] = true
					}
				}
				want := []byte{byte(seed), byte(seed >> 8), byte(seed >> 16), byte(seed >> 24)}
				if !bytes.Equal(out, want) {
					t.Fatalf("%08x: got %x want %x", seed, out, want)
				}
				for _, ok := range written {
					if !ok {
						t.Fatal("missing output byte")
					}
				}
			}
			opt(f)
			deadcode(f)
			lower(f)
			lateLower(f)
			deadcode(f)
			checkFunc(f)
			checkLower(f)
		})
	}
}

func TestNativeOptimizerArithmetic(t *testing.T) {
	for _, arch := range []string{"386", "amd64", "mips", "ppc64"} {
		for _, kind := range []string{"known", "cse"} {
			t.Run(fmt.Sprint(arch, "/", kind), func(t *testing.T) {
				c := loweringContext(arch)
				f, _ := genericFunc(c)
				ty := f.Config.Types
				b := f.Entry
				pos := b.Pos
				x := b.NewValue0(pos, OpArg, ty.Int32)
				var value *Value
				if kind == "known" {
					a := b.NewValue2(pos, OpAnd32, ty.Int32, x, f.ConstInt32(ty.Int32, 255))
					a = b.NewValue2(pos, OpOr32, ty.Int32, a, f.ConstInt32(ty.Int32, 256))
					value = b.NewValue2(pos, OpRsh32Ux32, ty.Int32, a, f.ConstInt32(ty.UInt32, 8))
				} else {
					ten := f.ConstInt32(ty.Int32, 10)
					a := b.NewValue2(pos, OpAdd32, ty.Int32, x, ten)
					aa := b.NewValue2(pos, OpAdd32, ty.Int32, ten, x)
					d := b.NewValue2(pos, OpSub32, ty.Int32, x, ten)
					dd := b.NewValue2(pos, OpSub32, ty.Int32, ten, x)
					v := b.NewValue2(pos, OpAdd32, ty.Int32, a, aa)
					v = b.NewValue2(pos, OpAdd32, ty.Int32, v, d)
					value = b.NewValue2(pos, OpSub32, ty.Int32, v, dd)
				}
				mem := b.NewValue0(pos, OpInitMem, c.Types.TypeMem)
				ptr := b.NewValue0I(pos, OpArg, ty.BytePtr, 16)
				b.SetControl(b.NewValue3A(pos, OpStore, c.Types.TypeMem, ty.Int32, ptr, value, mem))
				checkFunc(f)
				numberLines(f)
				if kind == "known" {
					knownBits(f)
				} else {
					cse(f)
				}
				deadcode(f)
				checkFunc(f)
				value = b.Controls[0].Args[1]
				if kind == "known" && value.Op != OpConst32 {
					t.Fatal("known bits did not fold")
				}
				for i := uint32(0); i < 1000; i++ {
					input := i * 0x9e3779b9
					want := uint32(1)
					if kind == "cse" {
						want = 4 * input
					}
					if got := evalGenericInteger(t, value, x, uint64(input)); uint32(got) != want {
						t.Fatalf("input %x got %x want %x", input, got, want)
					}
				}
			})
		}
	}
}

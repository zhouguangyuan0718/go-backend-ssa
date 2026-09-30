package ssa

import (
	"bytes"
	"go-backend-ssa/internal/types"
	"math"
	"testing"
)

func TestNativeSoftfloatStoredBits(t *testing.T) {
	for _, arch := range []string{"386", "amd64", "mips", "ppc64"} {
		c := loweringContext(arch)
		for _, size := range []int{4, 8} {
			for _, number := range []float64{0, math.Copysign(0, -1), 1.5, -2.5, math.Inf(1), math.Inf(-1), math.Float64frombits(0x7ff8123412345678)} {
				f, _ := genericFunc(c)
				f.Config.SoftFloat = true
				b := f.Entry
				ty := f.Config.Types
				typ, negOp, roundOp := ty.Float64, OpNeg64F, OpRound64F
				var constant *Value
				if size == 4 {
					typ = ty.Float32
					negOp = OpNeg32F
					roundOp = OpRound32F
					constant = f.ConstFloat32(typ, number)
				} else {
					constant = f.ConstFloat64(typ, number)
				}
				original := constant
				v := b.NewValue1(b.Pos, negOp, typ, constant)
				v = b.NewValue1(b.Pos, roundOp, typ, v)
				mem := b.NewValue0(b.Pos, OpInitMem, c.Types.TypeMem)
				ptr := b.NewValue0(b.Pos, OpArg, c.Types.NewPtr(typ))
				store := b.NewValue3A(b.Pos, OpStore, c.Types.TypeMem, typ, ptr, v, mem)
				b.SetControl(store)
				numberLines(f)
				softfloat(f)
				if original.InCache {
					t.Fatal("converted float remained in native constant cache")
				}
				opt(f)
				deadcode(f)
				checkFunc(f)
				got := make([]byte, size)
				written := make([]bool, size)
				var offset func(*Value) int64
				offset = func(p *Value) int64 {
					if p == ptr {
						return 0
					}
					switch p.Op {
					case OpCopy:
						return offset(p.Args[0])
					case OpOffPtr:
						return offset(p.Args[0]) + p.AuxInt
					}
					t.Fatalf("unexpected store address %s", p.LongString())
					return 0
				}
				order := c.IR.Base.Ctxt.Arch.ByteOrder
				for m := b.Controls[0]; m.Op != OpInitMem; m = m.Args[2] {
					if m.Op != OpStore {
						t.Fatalf("unexpected memory operation %s", m.LongString())
					}
					off := int(offset(m.Args[0]))
					width := int(m.Aux.(*types.Type).Size())
					bits := evalGenericInteger(t, m.Args[1], nil, 0)
					if off < 0 || off+width > size {
						t.Fatal("store outside result")
					}
					for i := off; i < off+width; i++ {
						if written[i] {
							t.Fatal("overlapping result stores")
						}
						written[i] = true
					}
					if width == 4 {
						order.PutUint32(got[off:], uint32(bits))
					} else if width == 8 {
						order.PutUint64(got[off:], bits)
					} else {
						t.Fatalf("unexpected store size %d", width)
					}
				}
				want := make([]byte, size)
				if size == 4 {
					order.PutUint32(want, math.Float32bits(float32(number))^0x80000000)
				} else {
					order.PutUint64(want, math.Float64bits(number)^0x8000000000000000)
				}
				if !bytes.Equal(got, want) {
					t.Fatalf("%s size=%d input=%x got=%x want=%x", arch, size, math.Float64bits(number), got, want)
				}
				for _, ok := range written {
					if !ok {
						t.Fatal("missing result byte")
					}
				}
				lower(f)
				lateLower(f)
				deadcode(f)
				checkFunc(f)
				checkLower(f)
			}
		}
	}
}

func TestNativeSoftfloatValueKinds(t *testing.T) {
	for _, arch := range []string{"386", "amd64", "mips", "ppc64"} {
		c := loweringContext(arch)
		f, _ := genericFunc(c)
		f.Config.SoftFloat = true
		e := f.Entry
		ty := f.Config.Types
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
		ptr := e.NewValue0I(pos, OpArg, ty.Float64Ptr, 8)
		dst := e.NewValue0I(pos, OpArg, ty.Float64Ptr, 16)
		arg := e.NewValue0I(pos, OpArg, ty.Float64, 24)
		load := l.NewValue2(pos, OpLoad, ty.Float64, ptr, mem)
		phi := j.NewValue2(pos, OpPhi, ty.Float64, load, arg)
		v := j.NewValue1(pos, OpNeg64F, ty.Float64, phi)
		v = j.NewValue1(pos, OpRound64F, ty.Float64, v)
		mem = j.NewValue3A(pos, OpStore, c.Types.TypeMem, ty.Float64, dst, v, mem)
		mem = j.NewValue2IA(pos, OpZero, c.Types.TypeMem, 8, ty.Float64, ptr, mem)
		mem = j.NewValue3I(pos, OpMove, c.Types.TypeMem, 8, dst, ptr, mem)
		mem.Aux = ty.Float64
		j.SetControl(mem)
		numberLines(f)
		softfloat(f)
		for _, b := range f.Blocks {
			for _, v := range b.Values {
				if v.Type.IsFloat() {
					t.Fatalf("float value survived: %s", v)
				}
				switch v.Op {
				case OpStore, OpZero, OpMove:
					if v.Aux.(*types.Type).IsFloat() {
						t.Fatal("float memory auxiliary type survived")
					}
				}
			}
		}
		opt(f)
		deadcode(f)
		f.invalidateCFG()
		checkFunc(f)
		lower(f)
		lateLower(f)
		deadcode(f)
		f.invalidateCFG()
		checkFunc(f)
		checkLower(f)
	}
}

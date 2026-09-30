package ssa

import (
	"math/bits"
	"testing"
)

// Interpret integer SSA operations independently of the rewrite rules. This is a
// test-only semantic oracle; it does not replace any production backend pass.
func evalGenericInteger(t *testing.T, root, input *Value, number uint64) uint64 {
	t.Helper()
	memo := map[*Value]uint64{input: number}
	var eval func(*Value) uint64
	eval = func(v *Value) uint64 {
		if n, ok := memo[v]; ok {
			return n
		}
		arg := func(i int) uint64 { return eval(v.Args[i]) }
		var n uint64
		switch v.Op {
		case OpConst8, OpConst16, OpConst32, OpConst64, OpConstBool:
			n = uint64(v.AuxInt)
		case OpCopy, OpTrunc64to32, OpTrunc32to8, OpTrunc64to8, OpCvtBoolToUint8:
			n = arg(0)
		case OpSignExt32to64:
			n = uint64(int64(int32(arg(0))))
		case OpZeroExt32to64:
			n = uint64(uint32(arg(0)))
		case OpZeroExt8to32, OpZeroExt8to64:
			n = uint64(uint8(arg(0)))
		case OpAdd32, OpAdd64:
			n = arg(0) + arg(1)
		case OpSub32, OpSub64:
			n = arg(0) - arg(1)
		case OpMul32, OpMul64:
			n = arg(0) * arg(1)
		case OpNeg32, OpNeg64:
			n = -arg(0)
		case OpAnd32, OpAnd64:
			n = arg(0) & arg(1)
		case OpOr32, OpOr64:
			n = arg(0) | arg(1)
		case OpXor32, OpXor64:
			n = arg(0) ^ arg(1)
		case OpCom32, OpCom64:
			n = ^arg(0)
		case OpLsh32x32, OpLsh32x64:
			n = uint64(uint32(arg(0)) << arg(1))
		case OpLsh64x32, OpLsh64x64:
			n = arg(0) << arg(1)
		case OpRsh32x32, OpRsh32x64:
			n = uint64(int64(int32(arg(0)) >> arg(1)))
		case OpRsh64x32, OpRsh64x64:
			n = uint64(int64(arg(0)) >> arg(1))
		case OpRsh32Ux32, OpRsh32Ux64:
			n = uint64(uint32(arg(0)) >> arg(1))
		case OpRsh64Ux32, OpRsh64Ux64:
			n = arg(0) >> arg(1)
		case OpRotateLeft32:
			n = uint64(bits.RotateLeft32(uint32(arg(0)), int(arg(1))))
		case OpRotateLeft64:
			n = bits.RotateLeft64(arg(0), int(arg(1)))
		case OpHmul32:
			n = uint64((int64(int32(arg(0))) * int64(int32(arg(1)))) >> 32)
		case OpHmul32u:
			n = (uint64(uint32(arg(0))) * uint64(uint32(arg(1)))) >> 32
		case OpHmul64u:
			n, _ = bits.Mul64(arg(0), arg(1))
		case OpHmul64:
			a, b := arg(0), arg(1)
			n, _ = bits.Mul64(a, b)
			if int64(a) < 0 {
				n -= b
			}
			if int64(b) < 0 {
				n -= a
			}
		case OpAvg32u:
			n = (uint64(uint32(arg(0))) + uint64(uint32(arg(1)))) >> 1
		case OpAvg64u:
			a, b := arg(0), arg(1)
			n = (a & b) + ((a ^ b) >> 1)
		case OpEq32:
			if uint32(arg(0)) == uint32(arg(1)) {
				n = 1
			}
		case OpEq64:
			if arg(0) == arg(1) {
				n = 1
			}
		case OpNeq32:
			if uint32(arg(0)) != uint32(arg(1)) {
				n = 1
			}
		case OpNeq64:
			if arg(0) != arg(1) {
				n = 1
			}
		case OpLeq32U:
			if uint32(arg(0)) <= uint32(arg(1)) {
				n = 1
			}
		case OpLeq64U:
			if arg(0) <= arg(1) {
				n = 1
			}
		case OpLess32U:
			if uint32(arg(0)) < uint32(arg(1)) {
				n = 1
			}
		case OpLess64U:
			if arg(0) < arg(1) {
				n = 1
			}
		default:
			t.Fatalf("unsupported oracle operation %s", v.LongString())
		}
		if size := v.Type.Size(); size < 8 {
			n &= (uint64(1) << (uint(size) * 8)) - 1
		}
		memo[v] = n
		return n
	}
	return eval(root)
}
func TestNativeGenericDivisionRewriteExecution(t *testing.T) {
	for _, arch := range []string{"386", "amd64"} {
		c := loweringContext(arch)
		for _, width := range []int{32, 64} {
			for _, unsigned := range []bool{false, true} {
				for _, divisibleTest := range []bool{false, true} {
					divisors := []uint64{1, 2, 3, 7, 8, 17, 0x7fffffff, 0x80000000, 0xffffffff, 0x7fffffffffffffff, 0x8000000000000000, 0xffffffffffffffff}
					for _, d := range divisors {
						if width == 32 {
							d = uint64(uint32(d))
						}
						if d == 0 {
							continue
						}
						f, _ := genericFunc(c)
						b := f.Entry
						ty := f.Config.Types
						typ := ty.Int32
						if width == 64 {
							typ = ty.Int64
						}
						if unsigned {
							typ = ty.UInt32
							if width == 64 {
								typ = ty.UInt64
							}
						}
						x := b.NewValue0(b.Pos, OpArg, typ)
						var divisor *Value
						if width == 32 {
							divisor = f.ConstInt32(typ, int32(d))
						} else {
							divisor = f.ConstInt64(typ, int64(d))
						}
						op := OpDiv32
						if width == 64 {
							op = OpDiv64
						}
						if unsigned {
							op = OpDiv32u
							if width == 64 {
								op = OpDiv64u
							}
						}
						if divisibleTest {
							op = OpMod32
							if width == 64 {
								op = OpMod64
							}
							if unsigned {
								op = OpMod32u
								if width == 64 {
									op = OpMod64u
								}
							}
						}
						result := b.NewValue2(b.Pos, op, typ, x, divisor)
						if divisibleTest {
							eq := OpEq32
							var zero *Value
							if width == 64 {
								eq = OpEq64
								zero = f.ConstInt64(typ, 0)
							} else {
								zero = f.ConstInt32(typ, 0)
							}
							result = b.NewValue2(b.Pos, eq, ty.Bool, result, zero)
						}
						mem := b.NewValue0(b.Pos, OpInitMem, c.Types.TypeMem)
						ptr := b.NewValue0I(b.Pos, OpArg, c.Types.NewPtr(result.Type), 16)
						store := b.NewValue3A(b.Pos, OpStore, c.Types.TypeMem, result.Type, ptr, result, mem)
						b.SetControl(store)
						checkFunc(f)
						numberLines(f)
						opt(f)
						divisible(f)
						divmod(f)
						opt(f)
						deadcode(f)
						checkFunc(f)
						root := store.Args[1]
						for _, v := range b.Values {
							switch v.Op {
							case OpDiv32, OpDiv32u, OpDiv64, OpDiv64u, OpMod32, OpMod32u, OpMod64, OpMod64u:
								t.Fatalf("constant division not rewritten: %s", v)
							}
						}
						inputs := []uint64{0, 1, 2, 3, 7, 0x7fffffff, 0x80000000, 0xffffffff, 0x7fffffffffffffff, 0x8000000000000000, 0xffffffffffffffff}
						state := uint64(123)
						for range 256 {
							state = state*6364136223846793005 + 1442695040888963407
							inputs = append(inputs, state)
						}
						for _, number := range inputs {
							if width == 32 {
								number = uint64(uint32(number))
							}
							var q, r uint64
							switch {
							case unsigned:
								q, r = number/d, number%d
							case width == 32:
								a, b := int32(number), int32(d)
								q, r = uint64(uint32(a/b)), uint64(uint32(a%b))
							default:
								a, b := int64(number), int64(d)
								q, r = uint64(a/b), uint64(a%b)
							}
							want := q
							if divisibleTest {
								want = 0
								if r == 0 {
									want = 1
								}
							}
							if got := evalGenericInteger(t, root, x, number); got != want {
								t.Fatalf("arch=%s bits=%d unsigned=%v divisible=%v x=%x d=%x got=%x want=%x\n%s", arch, width, unsigned, divisibleTest, number, d, got, want, f)
							}
						}
					}
				}
			}
		}
	}
}

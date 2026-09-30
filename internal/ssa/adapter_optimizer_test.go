package ssa

import (
	"bytes"
	"fmt"
	"math/rand"
	"slices"
	"sync"
	"testing"
)

// Each reported relation must hold in an independent concrete model. Nested
// checkpoints must restore all observable relations, including merged aliases.
func TestNativeOptimizerPosetUndo(t *testing.T) {
	for _, unsigned := range []bool{false, true} {
		c := newOpcodeContext()
		c.debugPoset = true
		po := newPoset()
		po.SetUnsigned(unsigned)
		const count = 16
		values := make([]*Value, count)
		for i := range values {
			values[i] = &Value{ID: ID(i + 1), Op: OpArg}
		}
		query := func() []bool {
			var out []bool
			for i, a := range values {
				for j, b := range values {
					if i == j {
						continue
					}
					got := []bool{po.Ordered(c, a, b), po.OrderedOrEqual(c, a, b), po.Equal(c, a, b), po.NonEqual(c, a, b)}
					want := []bool{i/2 < j/2, i/2 <= j/2, i/2 == j/2, i/2 != j/2}
					for k := range got {
						if got[k] && !want[k] {
							t.Fatalf("unsound relation %d(%d,%d)", k, i, j)
						}
					}
					out = append(out, got...)
				}
			}
			return out
		}
		rng := rand.New(rand.NewSource(137))
		po.Checkpoint()
		base := query()
		for round := 0; round < 40; round++ {
			before := query()
			po.Checkpoint()
			for step := 0; step < 24; step++ {
				i, j := rng.Intn(count), rng.Intn(count)
				if i == j {
					continue
				}
				if i/2 > j/2 {
					i, j = j, i
				}
				a, b := values[i], values[j]
				ok := false
				if i/2 == j/2 {
					ok = po.SetEqual(c, a, b)
				} else {
					switch rng.Intn(3) {
					case 0:
						ok = po.SetOrder(c, a, b)
					case 1:
						ok = po.SetOrderOrEqual(c, a, b)
					case 2:
						ok = po.SetNonEqual(c, a, b)
					}
				}
				if !ok {
					t.Fatal("consistent relation rejected")
				}
			}
			query()
			po.Undo(c)
			if !slices.Equal(before, query()) {
				t.Fatal("checkpoint restoration changed relations")
			}
			if !po.SetOrder(c, values[0], values[count-1]) {
				t.Fatal("base ordering rejected")
			}
		}
		po.Undo(c)
		if !slices.Equal(base, query()) {
			t.Fatal("outer restoration changed relations")
		}
		if err := po.CheckEmpty(); err != nil {
			t.Fatal(err)
		}
	}
}

func optimizerOrderGraph(c *Context, unsigned bool) *Func {
	f, _ := genericFunc(c)
	f.pass = &pass{name: "prove"}
	e := f.Entry
	ty := f.Config.Types
	pos := e.Pos
	mem := e.NewValue0(pos, OpInitMem, c.Types.TypeMem)
	ptr := e.NewValue0I(pos, OpArg, ty.BytePtr, 16)
	x := e.NewValue0I(pos, OpArg, ty.Int32, 0)
	y := e.NewValue0I(pos, OpArg, ty.Int32, 4)
	z := e.NewValue0I(pos, OpArg, ty.Int32, 8)
	less := OpLess32
	if unsigned {
		less = OpLess32U
	}
	b1, b2 := f.NewBlock(BlockIf), f.NewBlock(BlockIf)
	r0, r1, good, bad := f.NewBlock(BlockRet), f.NewBlock(BlockRet), f.NewBlock(BlockRet), f.NewBlock(BlockRet)
	for _, b := range []*Block{b1, b2, r0, r1, good, bad} {
		b.Pos = pos
	}
	e.Reset(BlockIf)
	e.SetControl(e.NewValue2(pos, less, ty.Bool, x, y))
	e.AddEdgeTo(b1)
	e.AddEdgeTo(r0)
	b1.SetControl(b1.NewValue2(pos, less, ty.Bool, y, z))
	b1.AddEdgeTo(b2)
	b1.AddEdgeTo(r1)
	b2.SetControl(b2.NewValue2(pos, less, ty.Bool, x, z))
	b2.AddEdgeTo(good)
	b2.AddEdgeTo(bad)
	for i, b := range []*Block{r0, r1, good, bad} {
		v := f.ConstInt32(ty.Int32, int32(i))
		b.SetControl(b.NewValue3A(pos, OpStore, c.Types.TypeMem, ty.Int32, ptr, v, mem))
	}
	return f
}
func evalOptimizerBranch(t *testing.T, f *Func, input [3]uint32) uint32 {
	t.Helper()
	var eval func(*Value) uint32
	eval = func(v *Value) uint32 {
		switch v.Op {
		case OpCopy:
			return eval(v.Args[0])
		case OpArg:
			return input[v.AuxInt/4]
		case OpConst32, OpConstBool:
			return uint32(v.AuxInt)
		case OpLess32:
			if int32(eval(v.Args[0])) < int32(eval(v.Args[1])) {
				return 1
			}
			return 0
		case OpLess32U:
			if eval(v.Args[0]) < eval(v.Args[1]) {
				return 1
			}
			return 0
		default:
			t.Fatalf("unexpected branch expression: %v", v.LongString())
			return 0
		}
	}
	b := f.Entry
	for step := 0; step < 20; step++ {
		switch b.Kind {
		case BlockPlain:
			b = b.Succs[0].b
		case BlockIf:
			edge := 1
			if eval(b.Controls[0]) != 0 {
				edge = 0
			}
			b = b.Succs[edge].b
		case BlockRet:
			mem := b.Controls[0]
			for mem.Op == OpCopy {
				mem = mem.Args[0]
			}
			if mem.Op != OpStore {
				t.Fatal("missing result store")
			}
			return eval(mem.Args[1])
		default:
			t.Fatal("unexpected block")
		}
	}
	t.Fatal("branch execution did not terminate")
	return 0
}
func TestNativeOptimizerProveSemantics(t *testing.T) {
	for _, arch := range []string{"386", "amd64", "mips", "ppc64", "arm64"} {
		for _, unsigned := range []bool{false, true} {
			t.Run(fmt.Sprint(arch, "/", unsigned), func(t *testing.T) {
				c := loweringContext(arch)
				c.checkEnabled = true
				c.debugPoset = true
				f := optimizerOrderGraph(c, unsigned)
				checkFunc(f)
				numberLines(f)
				prove(f)
				deadcode(f)
				f.invalidateCFG()
				checkFunc(f)
				for _, b := range f.Blocks {
					if b.Kind == BlockRet && b.Controls[0].Op == OpStore && b.Controls[0].Args[1].AuxInt == 3 {
						t.Fatal("transitive contradiction survived")
					}
				}
				rng := rand.New(rand.NewSource(532))
				cases := [][3]uint32{{0, 0, 0}, {0, 1, 2}, {0xffffffff, 0, 1}, {0x7fffffff, 0x80000000, 0xffffffff}, {2, 1, 0}}
				for range 1000 {
					cases = append(cases, [3]uint32{rng.Uint32(), rng.Uint32(), rng.Uint32()})
				}
				for _, in := range cases {
					less := func(a, b uint32) bool {
						if unsigned {
							return a < b
						}
						return int32(a) < int32(b)
					}
					want := uint32(0)
					if less(in[0], in[1]) {
						want = 1
						if less(in[1], in[2]) {
							want = 2
						}
					}
					if got := evalOptimizerBranch(t, f, in); got != want {
						t.Fatalf("%x: got %d want %d", in, got, want)
					}
				}
				if len(f.Cache.scrPoset) != 2 {
					t.Fatal("proof scratch sets not returned")
				}
				for _, po := range f.Cache.scrPoset {
					if err := po.CheckEmpty(); err != nil {
						t.Fatal(err)
					}
				}
				decomposeBuiltin(f)
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
}
func TestNativeOptimizerIsolation(t *testing.T) {
	run := func(c *Context) *Func {
		c.checkEnabled = true
		c.debugPoset = true
		f := optimizerOrderGraph(c, false)
		numberLines(f)
		prove(f)
		deadcode(f)
		checkFunc(f)
		f.pass = &pass{name: "generic cse", debug: 3}
		cse(f)
		return f
	}
	c := loweringContext("386")
	trace := new(bytes.Buffer)
	c.TraceWriter = trace
	old := run(c)
	before, logged := old.String(), trace.String()
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Go(func() {
			arch := []string{"amd64", "mips", "ppc64", "arm64"}[i%4]
			other := loweringContext(arch)
			f := run(other)
			if other.domainRelationTable == nil || f.Config.Types.Int.Size() != int64(other.Types.PointerSize()) {
				t.Error("missing instance state")
			}
			delete(other.domainRelationTable, OpLess32)
			other.debugPoset = false
			other.checkEnabled = false
		})
	}
	wg.Wait()
	if old.String() != before || trace.String() != logged || !c.debugPoset || !c.checkEnabled {
		t.Fatal("retained compilation changed")
	}
	if _, ok := c.domainRelationTable[OpLess32]; !ok {
		t.Fatal("shared relation table")
	}
	// The empty poset cache is reusable across compilations; it may not retain
	// the previous compilation's debug option or an owner pointer.
	shared := old.Cache.scrPoset
	other := loweringContext("amd64")
	f := optimizerOrderGraph(other, true)
	f.Cache.scrPoset = shared
	numberLines(f)
	prove(f)
	deadcode(f)
	checkFunc(f)
	for _, po := range f.Cache.scrPoset {
		if err := po.CheckEmpty(); err != nil {
			t.Fatal(err)
		}
	}
}

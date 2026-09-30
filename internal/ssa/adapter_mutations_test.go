package ssa

import (
	"math"
	"sync"
	"testing"

	"go-backend-ssa/internal/cmd/src"
	"go-backend-ssa/internal/types"
)

func TestNativeConstantCacheIsolation(t *testing.T) {
	retained, _ := diagnosticFixture(4)
	old := retained.ConstInt32(retained.Config.Types.Int, 37)
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			f, _ := diagnosticFixture(8)
			u := f.Config.Types.Int.Universe()
			for i := int64(-64); i < 64; i++ {
				v := f.ConstInt64(f.Config.Types.Int, i)
				if f.ConstInt64(f.Config.Types.Int, i) != v || !v.InCache || v.Block != f.Entry {
					t.Error("constant interning")
				}
			}
			for _, magic := range []int64{constSliceMagic, constInterfaceMagic, constNilMagic, constEmptyStringMagic} {
				f.ConstInt64(f.Config.Types.Int64, magic)
			}
			sliceA, sliceB := u.NewSlice(f.Config.Types.Int), u.NewSlice(f.Config.Types.Int)
			if f.ConstSlice(sliceA) != f.ConstSlice(sliceB) {
				t.Error("equivalent slice types did not share native constant")
			}
			p1, p2 := u.NewPtr(u.NewArray(f.Config.Types.Int, 3)), u.NewPtr(u.NewArray(f.Config.Types.Int, 3))
			if p1 == p2 || f.ConstNil(p1) != f.ConstNil(p2) {
				t.Error("structurally equal native types did not share constant")
			}
			pointer := u.NewPtr(f.Config.Types.Int)
			specials := []*Value{f.ConstSlice(sliceA), f.ConstInterface(u.NewInterface(nil)), f.ConstNil(pointer), f.ConstEmptyString(f.Config.Types.String)}
			for _, v := range specials {
				if !v.InCache || v.AuxInt != 0 {
					t.Error("special constant cache")
				}
				v.reset(OpUnknown)
				if v.InCache {
					t.Error("reset left cached constant")
				}
			}
			next := f.ConstNil(pointer)
			if next == specials[2] {
				t.Error("reset constant reused without allocation")
			}
			positive, negative := f.ConstFloat64(f.Config.Types.Float64, 0), f.ConstFloat64(f.Config.Types.Float64, math.Copysign(0, -1))
			if positive == negative || positive.AuxInt == negative.AuxInt {
				t.Error("signed zero lost identity")
			}
			if old.Type.Size() != 4 || old.AuxInt != 37 || !old.InCache {
				t.Error("retained constant changed")
			}
			checkNativeGraph(t, f)
		})
	}
	wg.Wait()
	if retained.ConstInt32(retained.Config.Types.Int, 37) != old {
		t.Fatal("old cache identity changed")
	}
}

func TestNativeValueMutationAndRecycling(t *testing.T) {
	f, _ := diagnosticFixture(8)
	typ := f.Config.Types.Int64
	pos := f.Entry.Pos.WithIsStmt()
	a, b := f.ConstInt64(typ, 2), f.ConstInt64(typ, 3)
	v := f.Entry.NewValue2(pos, OpAdd64, typ, a, b)
	destination := f.NewBlock(BlockPlain)
	copy := v.copyInto(destination)
	marked := v.copyIntoWithXPos(destination, pos.WithNotStmt())
	if copy.Pos.IsStmt() != src.PosNotStmt || marked.Pos.IsStmt() != src.PosIsStmt || copy.Args[0] != a || marked.Args[1] != b {
		t.Fatal("native copy position or arguments")
	}
	copy.copyOf(a)
	if copy.Op != OpCopy || copy.Type != a.Type || len(copy.Args) != 1 || copy.Args[0] != a {
		t.Fatal("copyOf")
	}
	copy.copyOf(copy)
	copy.reset(OpUnknown)
	if len(copy.Args) != 0 || copy.AuxInt != 0 || copy.Aux != nil {
		t.Fatal("reset")
	}
	checkNativeGraph(t, f)
	// Remove a value from its block before recycling, as the native pass does.
	destination.Values = destination.Values[1:]
	identity, id := copy, copy.ID
	f.freeValue(copy)
	fresh := destination.NewValue0(pos, OpConst64, typ)
	if fresh != identity || fresh.ID != id || fresh.Block != destination || fresh.Type != typ {
		t.Fatal("value free-list reuse")
	}
	isolated := f.NewBlock(BlockPlain)
	isolatedID := isolated.ID
	f.Blocks = f.Blocks[:len(f.Blocks)-1]
	f.freeBlock(isolated)
	newBlock := f.NewBlock(BlockIf)
	if newBlock != isolated || newBlock.ID != isolatedID || newBlock.Func != f || len(newBlock.Preds) != 0 || len(newBlock.Values) != 0 {
		t.Fatal("block free-list reuse")
	}
	// Recursive invalidation decrements each edge, including a repeated argument.
	one := f.Entry.NewValue0I(pos, OpConst64, typ, 1)
	two := f.Entry.NewValue2(pos.WithNotStmt(), OpAdd64, typ, one, one)
	if !two.invalidateRecursively() || one.Op != OpInvalid || two.Op != OpInvalid || one.Uses != 0 || len(two.Args) != 0 {
		t.Fatal("recursive invalidation")
	}
	checkNativeGraph(t, f)
}

func TestNativePhiAndCopyElimination(t *testing.T) {
	f, _ := diagnosticFixture(8)
	entry := f.Entry
	entry.Reset(BlockIf)
	left, right, join := f.NewBlock(BlockPlain), f.NewBlock(BlockPlain), f.NewBlock(BlockRet)
	entry.AddEdgeTo(left)
	entry.AddEdgeTo(right)
	left.AddEdgeTo(join)
	right.AddEdgeTo(join)
	a, b := f.ConstInt64(f.Config.Types.Int64, 5), f.ConstInt64(f.Config.Types.Int64, 9)
	phi := join.NewValue2(entry.Pos, OpPhi, a.Type, a, b)
	join.SetControl(phi)
	entry.Likely = BranchLikely
	entry.swapSuccessors()
	if entry.Likely != BranchUnlikely || entry.Succs[0].b != right || right.Preds[0].i != 0 {
		t.Fatal("successor swap")
	}
	right.removeSucc(0)
	join.removePred(1)
	join.removePhiArg(phi, 1)
	if phi.Op != OpCopy || len(phi.Args) != 1 || phi.Args[0] != a || b.Uses != 0 {
		t.Fatal("phi elimination after predecessor removal")
	}
	copy := join.NewValue1(entry.Pos, OpCopy, a.Type, phi)
	sum := join.NewValue2(entry.Pos, OpAdd64, a.Type, copy, copy)
	join.SetControl(sum)
	u := a.Type.Universe()
	empty := join.NewValue1(entry.Pos, OpCopy, u.NewArray(a.Type, 0), copy)
	slot := &LocalSlot{Type: a.Type}
	f.Names = []*LocalSlot{slot}
	f.NamedValues[*slot] = []*Value{copy}
	copyelim(f)
	if sum.Args[0] != a || sum.Args[1] != a || f.NamedValues[*slot][0] != a || empty.Op != OpEmpty {
		t.Fatal("native combined copy/phi pass")
	}
	checkNativeGraph(t, f)
	// Self-only phis and unreachable copy cycles retain native termination rules.
	dead := f.NewBlock(BlockPlain)
	self := dead.NewValue0(entry.Pos, OpPhi, a.Type)
	self.AddArg(self)
	if phielimValue(self) {
		t.Fatal("self-only phi eliminated")
	}
	x := dead.NewValue0(entry.Pos, OpCopy, a.Type)
	y := dead.NewValue1(entry.Pos, OpCopy, a.Type, x)
	x.AddArg(y)
	root := copySource(x)
	if root.Op != OpUnknown {
		t.Fatal("copy cycle did not terminate at Unknown")
	}
	checkNativeGraph(t, f)
}

func TestNativePositionTypeOwnership(t *testing.T) {
	f, _ := diagnosticFixture(8)
	u := f.Config.Types.Int.Universe()
	b := f.NewBlock(BlockPlain)
	v := b.NewValue0(src.XPos{}, OpCopy, u.TypeMem)
	if !v.LackingPos() || !b.LackingPos() {
		t.Fatal("memory copy position")
	}
	detached := &Value{Op: OpCopy, Type: u.TypeMem}
	if !detached.LackingPos() {
		t.Fatal("detached memory owner")
	}
	v.Op = OpVarDef
	v.Type = nil
	if !v.LackingPos() {
		t.Fatal("position-independent operation with graph owner")
	}
	v.Op = OpCopy
	v.Type = u.Types[types.TINT]
	if v.LackingPos() || b.LackingPos() {
		t.Fatal("ordinary copy position")
	}
	foreign, _ := auxFixture(4)
	v.Type = foreign.TypeMem
	defer func() {
		if recover() == nil {
			t.Fatal("foreign type accepted")
		}
	}()
	v.LackingPos()
}

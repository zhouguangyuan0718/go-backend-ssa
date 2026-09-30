package ssa

import (
	"bytes"
	"fmt"
	"strings"
	"sync"
	"testing"

	"go-backend-ssa/internal/cmd/src"
)

type deadcodeFrontend struct {
	*diagnosticFrontend
	warnings []string
}

func (f *deadcodeFrontend) Warnl(pos src.XPos, format string, args ...any) {
	f.warnings = append(f.warnings, fmt.Sprintf(format, args...))
}

func deadcodeFixture(size int) (*Func, *Value, *Value, *Value, *Block, *bytes.Buffer) {
	f, fe := diagnosticFixture(size)
	f.fe = &deadcodeFrontend{diagnosticFrontend: fe}
	e := f.Entry
	e.Reset(BlockFirst)
	join, dead := f.NewBlock(BlockRet), f.NewBlock(BlockPlain)
	e.AddEdgeTo(join)
	e.AddEdgeTo(dead)
	dead.AddEdgeTo(join)
	file := src.NewFileBase("deadcode.go", "deadcode.go")
	pos := func(line uint) src.XPos { return f.Config.ctxt.PosTable.XPos(src.MakePos(file, line, 2)) }
	e.Pos = pos(10)
	join.Pos = pos(30)
	dead.Pos = pos(40)
	it := f.Config.Types.Int64
	u := it.Universe()
	a, b := f.ConstInt64(it, 7), f.ConstInt64(it, 13)
	f.ConstInt64(it, 42)
	mem := e.NewValue0(pos(10), OpInitMem, u.TypeMem)
	sp := e.NewValue0(pos(10), OpSP, f.Config.Types.Uintptr)
	ptr := e.NewValue1I(pos(10), OpOffPtr, u.NewPtr(it), 8, sp)
	gone := e.NewValue2(pos(15), OpAdd64, it, a, b)
	live := e.NewValue2(pos(15), OpSub64, it, b, a)
	deadValue := dead.NewValue2(pos(40), OpMul64, it, a, b)
	phi := join.NewValue2(pos(25), OpPhi, it, live, deadValue)
	store := join.NewValue3A(pos(25), OpStore, u.TypeMem, it, ptr, phi, mem)
	join.SetControl(store)
	tail := join.NewValue2(pos(30), OpAdd64, it, a, b)
	liveSlot, deadSlot := &LocalSlot{Type: it}, &LocalSlot{Type: it, Off: 8}
	f.Names = []*LocalSlot{liveSlot, deadSlot}
	f.NamedValues[*liveSlot] = []*Value{live, live, gone}
	f.NamedValues[*deadSlot] = []*Value{gone, deadValue}
	trace := new(bytes.Buffer)
	f.Config.Types.compilation.TraceWriter = trace
	f.pass.debug = 1
	return f, gone, live, tail, dead, trace
}

func TestNativeDeadcodeAndNumberLinesIsolation(t *testing.T) {
	run := func(size int) *Func {
		f, gone, live, tail, dead, trace := deadcodeFixture(size)
		numberLines(f)
		if gone.Pos.IsStmt() != src.PosIsStmt || live.Pos.IsStmt() == src.PosIsStmt || tail.Pos.IsStmt() != src.PosIsStmt {
			t.Error("numberLines did not select original statement boundaries")
		}
		if !strings.Contains(trace.String(), "Mark stmt") {
			t.Error("debug trace not instance bound")
		}
		deadcode(f)
		checkNativeGraph(t, f)
		if live.Pos.IsStmt() != src.PosIsStmt || f.Blocks[1].Pos.IsStmt() != src.PosIsStmt {
			t.Error("deleted statement boundary not conserved")
		}
		if gone.Block != nil || tail.Block != nil || dead.Func != nil || len(f.Blocks) != 2 || f.Entry.Kind != BlockPlain || len(f.Blocks[1].Preds) != 1 {
			t.Error("dead graph not reclaimed")
		}
		if len(f.Names) != 1 || len(f.NamedValues) != 1 || len(f.NamedValues[*f.Names[0]]) != 1 || f.NamedValues[*f.Names[0]][0] != live {
			t.Error("named values not deduplicated")
		}
		if len(f.constants[42]) != 0 {
			t.Error("dead cached value retained")
		}
		before := f.String()
		deadcode(f)
		checkNativeGraph(t, f)
		if f.String() != before {
			t.Error("deadcode is not stable")
		}
		return f
	}
	retained := run(4)
	before := retained.String()
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			f := run(8)
			if f.Config.Types.Int.Size() != 8 || retained.Config.Types.Int.Size() != 4 {
				t.Error("target state leaked")
			}
		})
	}
	wg.Wait()
	if retained.String() != before {
		t.Fatal("old graph changed")
	}
}

func TestNativeDeadcodeRejectsRegallocBeforeMutation(t *testing.T) {
	f, _, _, _, _, _ := deadcodeFixture(8)
	numberLines(f)
	f.RegAlloc = make([]Location, f.NumValues())
	before := f.String()
	defer func() {
		failure := recover()
		if failure == nil || !strings.Contains(failure.(string), "deadcode after regalloc") {
			t.Fatalf("unexpected failure %v", failure)
		}
		if f.String() != before {
			t.Fatal("rejected graph mutated")
		}
	}()
	deadcode(f)
}

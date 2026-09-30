package ssa

import (
	"sync"
	"testing"
)

func TestNativeOpcodeQueries(t *testing.T) {
	c := newOpcodeContext()
	for i, info := range c.opcodeTable {
		op := Op(i)
		if op.String() != info.name || op.Asm() != info.asm || op.Scale() != int16(info.scale) || op.SymEffect() != info.symEffect || op.IsCall() != info.call || op.IsTailCall() != info.tailCall || op.HasSideEffects() != info.hasSideEffects || op.UnsafePoint() != info.unsafePoint || op.ResultInArg0() != info.resultInArg0 {
			t.Fatalf("native opcode queries differ at %d", i)
		}
	}
	for _, op := range []Op{-1, Op(len(c.opcodeTable))} {
		func() {
			defer func() {
				if recover() == nil {
					t.Error("accepted out-of-range opcode", int(op))
				}
			}()
			_ = op.String()
		}()
	}
	if n := testing.AllocsPerRun(100, func() { op := OpAMD64ADDQ; _ = op.String(); _ = op.Asm(); _ = op.Scale(); _ = op.IsCall() }); n != 0 {
		t.Fatal("opcode query allocates", n)
	}
}

func TestOpcodeContextIsolation(t *testing.T) {
	old := newOpcodeContext()
	wb := old.opcodeTable[Op386LoweredWB].reg.clobbers
	inOp, outOp := -1, -1
	for i, p := range old.opcodeTable {
		if inOp < 0 && len(p.reg.inputs) > 0 {
			inOp = i
		}
		if outOp < 0 && len(p.reg.outputs) > 0 {
			outOp = i
		}
	}
	if inOp < 0 || outOp < 0 {
		t.Fatal("missing native register constraints")
	}
	input := old.opcodeTable[inOp].reg.inputs[0]
	output := old.opcodeTable[outOp].reg.outputs[0]
	registerName := old.registersAMD64[0].name
	paramReg := old.paramIntRegAMD64[0]
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			c := newOpcodeContext()
			// The real NewConfig uses this exact native mutation for shared mode.
			c.opcodeTable[Op386LoweredWB].reg.clobbers = c.opcodeTable[Op386LoweredWB].reg.clobbers.addReg(3)
			if !c.opcodeTable[Op386LoweredWB].reg.clobbers.hasReg(3) {
				t.Error("missing shared-mode clobber")
			}
			c.opcodeTable[inOp].reg.inputs[0].regs = c.opcodeTable[inOp].reg.inputs[0].regs.addReg(127)
			c.opcodeTable[outOp].reg.outputs[0].regs = c.opcodeTable[outOp].reg.outputs[0].regs.addReg(126)
			c.registersAMD64[0].name = "other"
			c.paramIntRegAMD64[0] = 127
			if old.opcodeTable[Op386LoweredWB].reg.clobbers != wb || old.opcodeTable[inOp].reg.inputs[0] != input || old.opcodeTable[outOp].reg.outputs[0] != output || old.registersAMD64[0].name != registerName || old.paramIntRegAMD64[0] != paramReg {
				t.Error("shared opcode/register backing storage")
			}
			if Op386LoweredWB.String() != c.opcodeTable[Op386LoweredWB].name {
				t.Error("scalar query depends on another instance")
			}
		})
	}
	wg.Wait()
	fresh := newOpcodeContext()
	if fresh.opcodeTable[Op386LoweredWB].reg.clobbers != wb {
		t.Fatal("new instance inherited shared-mode mutation")
	}
	u := universe(8)
	bundle := NewTypes(u)
	if bundle.compilation.opcodeTables != nil || typeContextOf(bundle.Int).opcodeTables != nil {
		t.Fatal("type-only queries initialize large backend tables")
	}
}

func BenchmarkNativeOpcodeQueries(b *testing.B) {
	var checksum uint64
	for i := 0; b.Loop(); i++ {
		op := Op(i % int(OpternUint64x8+1))
		checksum += uint64(len(op.String())) + uint64(op.Asm()) + uint64(op.Scale())
		if op.IsCall() {
			checksum++
		}
	}
	if checksum == 0 {
		b.Fatal("opcode query results were not consumed")
	}
}

package ssa

import (
	"fmt"
	"go-backend-ssa/internal/types"
	"io"
	"os/exec"
)

// This adapter binds native SSA type operations to their compilation's universe.
// It introduces no separate public session or process-global state.
type Context struct {
	*opcodeTables
	Types       *types.Universe
	ptrSize     int
	TraceWriter io.Writer
	Command     func(string, ...string) *exec.Cmd
}

func typeContextOf(t *types.Type) *Context {
	if t == nil || t.Universe() == nil {
		panic("SSA type predicate requires an initialized compiler type")
	}
	u := t.Universe()
	return &Context{Types: u, ptrSize: u.PointerSize()}
}

// NewTypes populates the native SSA type collection from an initialized universe.
// Calls that share a universe must be serialized, like other type operations.
func NewTypes(u *types.Universe) *Types {
	c := &Context{Types: u}
	return c.NewTypes()
}

func (c *Context) checkTypes() {
	if c.Types == nil {
		panic("SSA types require an initialized universe")
	}
	// Validate before the native initializer writes fields or interns pointers.
	// The public universe fields can be modified by callers.
	for _, k := range []types.Kind{types.TBOOL, types.TINT8, types.TINT16, types.TINT32, types.TINT64,
		types.TUINT8, types.TUINT16, types.TUINT32, types.TUINT64, types.TINT,
		types.TFLOAT32, types.TFLOAT64, types.TUINT, types.TUINTPTR, types.TSTRING} {
		t := c.Types.Types[k]
		if t == nil || t.Universe() != c.Types || t.Kind() != k {
			panic("SSA builtin type belongs to another or uninitialized universe")
		}
	}
	for _, t := range []*types.Type{c.Types.TypeVec128, c.Types.TypeVec256, c.Types.TypeVec512, c.Types.TypeMask} {
		if t == nil || t.Universe() != c.Types {
			panic("SSA vector type belongs to another or uninitialized universe")
		}
	}
}

// opcodeTables owns the unmodified native table initializers. Keeping this
// storage behind a pointer avoids making CanSSA and NewTypes allocate backend
// tables they do not use. Only SSA construction initializes these tables.
type opcodeTables struct {
	opcodeTable            [OpternUint64x8 + 1]opInfo
	registers386           [17]Register
	paramIntReg386         []int8
	paramFloatReg386       []int8
	gpRegMask386           regMask
	fpRegMask386           regMask
	specialRegMask386      regMask
	framepointerReg386     int8
	linkReg386             int8
	registersAMD64         [57]Register
	paramIntRegAMD64       []int8
	paramFloatRegAMD64     []int8
	gpRegMaskAMD64         regMask
	fpRegMaskAMD64         regMask
	simdRegMaskAMD64       regMask
	specialRegMaskAMD64    regMask
	framepointerRegAMD64   int8
	linkRegAMD64           int8
	registersARM           [33]Register
	paramIntRegARM         []int8
	paramFloatRegARM       []int8
	gpRegMaskARM           regMask
	fpRegMaskARM           regMask
	specialRegMaskARM      regMask
	framepointerRegARM     int8
	linkRegARM             int8
	registersARM64         [64]Register
	paramIntRegARM64       []int8
	paramFloatRegARM64     []int8
	gpRegMaskARM64         regMask
	fpRegMaskARM64         regMask
	simdRegMaskARM64       regMask
	specialRegMaskARM64    regMask
	framepointerRegARM64   int8
	linkRegARM64           int8
	registersLOONG64       [63]Register
	paramIntRegLOONG64     []int8
	paramFloatRegLOONG64   []int8
	gpRegMaskLOONG64       regMask
	fpRegMaskLOONG64       regMask
	specialRegMaskLOONG64  regMask
	framepointerRegLOONG64 int8
	linkRegLOONG64         int8
	registersMIPS          [48]Register
	paramIntRegMIPS        []int8
	paramFloatRegMIPS      []int8
	gpRegMaskMIPS          regMask
	fpRegMaskMIPS          regMask
	specialRegMaskMIPS     regMask
	framepointerRegMIPS    int8
	linkRegMIPS            int8
	registersMIPS64        [63]Register
	paramIntRegMIPS64      []int8
	paramFloatRegMIPS64    []int8
	gpRegMaskMIPS64        regMask
	fpRegMaskMIPS64        regMask
	specialRegMaskMIPS64   regMask
	framepointerRegMIPS64  int8
	linkRegMIPS64          int8
	registersPPC64         [64]Register
	paramIntRegPPC64       []int8
	paramFloatRegPPC64     []int8
	gpRegMaskPPC64         regMask
	fpRegMaskPPC64         regMask
	specialRegMaskPPC64    regMask
	framepointerRegPPC64   int8
	linkRegPPC64           int8
	registersRISCV64       [64]Register
	paramIntRegRISCV64     []int8
	paramFloatRegRISCV64   []int8
	gpRegMaskRISCV64       regMask
	fpRegMaskRISCV64       regMask
	specialRegMaskRISCV64  regMask
	framepointerRegRISCV64 int8
	linkRegRISCV64         int8
	registersS390X         [33]Register
	paramIntRegS390X       []int8
	paramFloatRegS390X     []int8
	gpRegMaskS390X         regMask
	fpRegMaskS390X         regMask
	specialRegMaskS390X    regMask
	framepointerRegS390X   int8
	linkRegS390X           int8
	registersWasm          [67]Register
	paramIntRegWasm        []int8
	paramFloatRegWasm      []int8
	gpRegMaskWasm          regMask
	fpRegMaskWasm          regMask
	fp32RegMaskWasm        regMask
	fp64RegMaskWasm        regMask
	simdRegMaskWasm        regMask
	specialRegMaskWasm     regMask
	framepointerRegWasm    int8
	linkRegWasm            int8
}

func newOpcodeContext() *Context {
	c := &Context{opcodeTables: new(opcodeTables)}
	c.initialize()
	return c
}

// AuxCall already retains its ABI configuration. Derive type ownership there so
// zero-result late calls use the right memory type without another owner field.
func auxCallContext(a *AuxCall) *Context {
	if a == nil || a.abiInfo == nil {
		panic("SSA call has no ABI analysis")
	}
	u := a.abiInfo.Config().Universe()
	if u == nil {
		panic("SSA call ABI has no compilation owner")
	}
	return &Context{Types: u}
}

// Diagnostics reuse the type collection's existing compilation owner. A nil
// writer or disabled graph renderer may return before needing any host input.
func diagnosticContext(f *Func) *Context {
	if f != nil && f.Config != nil && f.Config.Types.compilation != nil {
		return f.Config.Types.compilation
	}
	return &Context{}
}
func htmlWriterContext(w *HTMLWriter) *Context {
	if w == nil {
		return &Context{}
	}
	return diagnosticContext(w.Func)
}
func (c *Context) traceWriter() io.Writer {
	if c.TraceWriter != nil {
		return c.TraceWriter
	}
	return io.Discard
}
func (c *Context) tracePrintf(format string, args ...any) (int, error) {
	return fmt.Fprintf(c.traceWriter(), format, args...)
}
func (c *Context) tracePrintln(args ...any) (int, error) {
	return fmt.Fprintln(c.traceWriter(), args...)
}
func (c *Context) command(name string, args ...string) *exec.Cmd {
	if c.Command == nil {
		panic("SSA graph rendering requires a command provider")
	}
	return c.Command(name, args...)
}

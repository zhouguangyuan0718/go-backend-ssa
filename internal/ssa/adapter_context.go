package ssa

import (
	"encoding/binary"
	"fmt"
	"go-backend-ssa/internal/abi"
	"go-backend-ssa/internal/cmd/obj"
	"go-backend-ssa/internal/cmd/src"
	"go-backend-ssa/internal/cmd/sys"
	"go-backend-ssa/internal/go/buildcfg"
	"go-backend-ssa/internal/ir"
	"go-backend-ssa/internal/logopt"
	"go-backend-ssa/internal/reflectdata"
	"go-backend-ssa/internal/types"
	"io"
	"io/fs"
	"os/exec"
	"runtime"
	"time"
)

// This adapter binds native SSA type operations to their compilation's universe.
// It introduces no separate public session or process-global state.
type Context struct {
	BlockStart, BlockEnd, FuncEnd *Value
	// Host services are explicit; no process environment or filesystem fallback.
	Now               func() time.Time
	ReadMemStats      func(*runtime.MemStats)
	DumpDirectory     string
	CreateDump        func(string) (io.WriteCloser, error)
	OpenHTML          func(string, int, fs.FileMode) (io.WriteCloser, error)
	WorkingDirectory  func() (string, error)
	LookPath          func(string) (string, error)
	checkRandSeed     int
	IntrinsicsDebug   int
	IntrinsicsDisable bool
	BuildDebug        int
	BuildTest         int
	BuildStats        int
	BuildDump         map[string]bool
	GenssaDump        map[string]bool
	passes            [59]pass
	passOrder         [42]constraint
	archWasm          *sys.Arch
	noRegisters       [32]register
	pairableLoads     map[Op]pairableLoadInfo
	pairableStores    map[Op]pairableStoreInfo
	archRISCV64       *sys.Arch
	combineFirst      map[Op]bool
	combine           map[[2]Op]Op
	needSplit         map[Op]bool
	faultOnLoad       bool
	tmpVal            Value
	targetOS          string
	// FramePointerEnabled is the native platform frame-pointer policy supplied by the driver.
	FramePointerEnabled bool
	checkpointBound     limitFact
	checkEnabled        bool
	debugPoset          bool
	domainRelationTable map[Op]struct {
		d domain
		r relation
	}
	*opcodeTables
	Types                       *types.Universe
	IR                          *ir.Context
	ABI                         *abi.Context
	ReflectData                 *reflectdata.Context
	Log                         *logopt.Context
	GOAMD64, GOPPC64, GORISCV64 int
	GOARM                       buildcfg.GoarmFeatures
	AuxMark                     auxMark
	ptrSize                     int
	TraceWriter                 io.Writer
	Command                     func(string, ...string) *exec.Cmd
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
func (c *Context) panicLogf(format string, args ...any) {
	message := fmt.Sprintf(format, args...)
	fmt.Fprintln(c.traceWriter(), message)
	panic(message)
}
func (c *Context) command(name string, args ...string) *exec.Cmd {
	if c.Command == nil {
		panic("SSA graph rendering requires a command provider")
	}
	return c.Command(name, args...)
}

// Values normally retain their graph's type collection; detached initialized
// values can still identify their type universe directly.
func valueContext(v *Value) *Context {
	if v != nil && v.Block != nil && v.Block.Func != nil && v.Block.Func.Config != nil {
		if c := v.Block.Func.Config.Types.compilation; c != nil {
			if v.Type != nil && v.Type.Universe() != c.Types {
				panic("SSA value belongs to another type universe")
			}
			return c
		}
	}
	if v == nil {
		panic("SSA value has no type owner")
	}
	return typeContextOf(v.Type)
}

// Names retain their existing IR owner, which owns merge policy and target flags.
func nameContext(n *ir.Name) *Context {
	graph := ir.ContextOf(n)
	if n.Type() != nil && n.Type().Universe() != graph.Types {
		panic("SSA local belongs to another type universe")
	}
	return &Context{Types: graph.Types, IR: graph, ptrSize: graph.Types.PointerSize()}
}
func (c *Context) tracePrint(args ...any) (int, error) { return fmt.Fprint(c.traceWriter(), args...) }

func configContext(c *Config) *Context {
	if c == nil || c.Types.compilation == nil {
		panic("SSA configuration has no compilation owner")
	}
	return c.Types.compilation
}
func (c *Context) fatalf(format string, args ...any) { panic(fmt.Errorf(format, args...)) }

// NewContext binds lowering to existing compiler services. Configure the native
// GOAMD64, GOARM, GOPPC64 or GORISCV64 fields before NewConfig for that target.
// Calls within this context are serialized; independent contexts share no tables.
func NewContext(a *abi.Context, rd *reflectdata.Context, log *logopt.Context) *Context {
	c := &Context{ABI: a, ReflectData: rd, Log: log}
	if a != nil {
		c.IR = a.IR
	}
	if c.IR != nil {
		c.Types = c.IR.Types
	}
	c.checkInputs()
	c.checkTypes()
	c.ptrSize = c.Types.PointerSize()
	c.targetOS = c.IR.Base.Ctxt.TargetOS
	if archs := sys.ContextOfArch(c.IR.Base.Ctxt.Arch.Arch); archs != nil {
		c.archRISCV64 = archs.ArchRISCV64
		c.archWasm = archs.ArchWasm
	}
	c.opcodeTables = new(opcodeTables)
	c.initialize()
	c.initializePhases()
	return c
}

func (c *Context) checkInputs() {
	if c.IR == nil || c.IR.Base == nil || c.Types == nil || c.Types != c.IR.Types || c.ABI == nil || c.ABI.IR != c.IR || c.ReflectData == nil || c.ReflectData.Typecheck == nil || c.ReflectData.Typecheck.IR != c.IR || c.Log == nil {
		panic("SSA lowering requires services from the same compilation")
	}
	rt := c.ReflectData.Runtime
	if rt == nil || rt.Type == nil || rt.PtrType == nil || rt.Types != c.Types || rt.Writer == nil || rt.Writer.Link != c.IR.Base.Ctxt || rt.Writer.Types != c.Types {
		panic("SSA lowering requires matching runtime layouts and object writer")
	}
	link := c.IR.Base.Ctxt
	if link == nil || link.Arch == nil || link.Arch.Arch == nil || link.Arch.ByteOrder == nil || link.TargetOS == "" || c.Types.PosTable != &link.PosTable || link.Arch.PtrSize != c.Types.PointerSize() || link.Arch.RegSize != c.Types.RegisterSize() {
		panic("SSA lowering requires an explicit matching target")
	}
}
func (c *Context) checkConfig(arch string, ts Types, link *obj.Link, optimize, softfloat bool) {
	c.checkInputs()
	if link != c.IR.Base.Ctxt || c.targetOS != link.TargetOS || c.opcodeTables == nil || ts.compilation != c || link.Arch.Name != arch {
		panic("SSA configuration requires matching compiler inputs")
	}
	// Validate the complete native collection, including all pointer fields.
	if ts != *c.NewTypes() {
		panic("SSA configuration requires canonical types from this compilation")
	}
	size := 8
	switch arch {
	case "386", "arm", "mips", "mipsle":
		size = 4
	case "amd64", "arm64", "ppc64", "ppc64le", "mips64", "mips64le", "loong64", "riscv64", "s390x", "wasm":
	default:
		panic("unsupported SSA architecture: " + arch)
	}
	if c.Types.PointerSize() != size || c.Types.RegisterSize() != size {
		panic("SSA target layout does not match architecture")
	}
	var order binary.ByteOrder = binary.LittleEndian
	switch arch {
	case "mips", "mips64", "ppc64", "s390x":
		order = binary.BigEndian
	}
	if link.Arch.ByteOrder.Uint32([]byte{1, 2, 3, 4}) != order.Uint32([]byte{1, 2, 3, 4}) {
		panic("SSA target byte order does not match architecture")
	}

	switch arch {
	case "amd64":
		if c.GOAMD64 < 1 || c.GOAMD64 > 4 {
			panic("SSA lowering requires explicit GOAMD64 v1 through v4")
		}
	case "arm":
		if c.GOARM.Version < 5 || c.GOARM.Version > 7 || c.GOARM.SoftFloat != softfloat {
			panic("SSA lowering requires matching GOARM version and float mode")
		}
	case "ppc64", "ppc64le":
		if c.GOPPC64 < 8 || c.GOPPC64 > 10 {
			panic("SSA lowering requires explicit GOPPC64 power8 through power10")
		}
	case "riscv64":
		if c.GORISCV64 != 20 && c.GORISCV64 != 22 && c.GORISCV64 != 23 {
			panic("SSA lowering requires explicit GORISCV64 rva20, rva22 or rva23")
		}
	}
}

func (c *Context) fatalfAt(pos src.XPos, format string, args ...any) {
	panic(fmt.Errorf("%s: %s", c.IR.Base.FmtPos(pos), fmt.Sprintf(format, args...)))
}

// Preserve the native TempAt operation after validating the frontend's function
// and type. A foreign frontend must not append temporaries to another graph.
func (c *Context) tempAt(pos src.XPos, fn *ir.Func, typ *types.Type) *ir.Name {
	if fn != nil && ir.ContextOf(fn) != c.IR {
		panic("SSA temporary belongs to another compiler function")
	}
	if typ != nil && typ.Universe() != c.Types {
		panic("SSA temporary type belongs to another universe")
	}
	return c.ReflectData.Typecheck.TempAt(pos, fn, typ)
}

func (c *Context) now() time.Time {
	if c.Now == nil {
		panic("SSA compilation requires a clock provider")
	}
	return c.Now()
}
func (c *Context) readMemStats(stats *runtime.MemStats) {
	if c.ReadMemStats == nil {
		panic("SSA memory statistics require a metric provider")
	}
	c.ReadMemStats(stats)
}
func (c *Context) dumpEnvironment(key string) string {
	if key != "GOSSADIR" {
		panic("unsupported SSA environment input")
	}
	return c.DumpDirectory
}
func (c *Context) createDump(name string) (io.WriteCloser, error) {
	if c.CreateDump == nil {
		panic("SSA dumping requires an output provider")
	}
	return c.CreateDump(name)
}

func (c *Context) openHTML(path string, flags int, mode fs.FileMode) (io.WriteCloser, error) {
	if c.OpenHTML == nil {
		panic("SSA HTML output requires an output provider")
	}
	return c.OpenHTML(path, flags, mode)
}
func (c *Context) workingDirectory() (string, error) {
	if c.WorkingDirectory == nil {
		panic("SSA HTML output requires a working directory provider")
	}
	return c.WorkingDirectory()
}
func (c *Context) lookPath(name string) (string, error) {
	if name != "dot" {
		panic("unsupported SSA executable input")
	}
	if c.LookPath == nil {
		panic("SSA graph output requires an executable lookup provider")
	}
	return c.LookPath(name)
}

// BindFuncDebug retains the owner of a native debug result while preserving its
// fields. It also supports the native zero-valued result before SSA construction.
func (c *Context) BindFuncDebug(v FuncDebug) FuncDebug {
	if v.compilation != nil && v.compilation != c {
		panic("debug result belongs to another compilation")
	}
	v.compilation = c
	return v
}

// BoundsKind.Code needs only the stateless native fatal diagnostic.
func boundsKindContext(BoundsKind) *Context { return &Context{} }

func (c *Context) checkFuncDebug(ctxt *obj.Link, f *Func, level int, offset func(LocalSlot) int32, result *FuncDebug) {
	if ctxt != c.IR.Base.Ctxt || result == nil || result.compilation != nil && result.compilation != c {
		panic("debug result or link belongs to another compilation")
	}
	result.compilation = c
}
func (c *Context) checkFuncDebugNoOptimized(ctxt *obj.Link, f *Func, logging bool, offset func(LocalSlot) int32, result *FuncDebug) {
	c.checkFuncDebug(ctxt, f, 0, offset, result)
}

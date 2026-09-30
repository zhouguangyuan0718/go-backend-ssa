package ssa

import (
	"fmt"
	"go-backend-ssa/internal/cmd/obj"
	"go-backend-ssa/internal/cmd/src"
	"go-backend-ssa/internal/ir"
	"go-backend-ssa/internal/types"
	"sync"
	"testing"
)

type callsFrontend struct {
	*genericFrontend
	fn   *ir.Func
	link *obj.Link
	wb   bool
}

func (fe *callsFrontend) Func() *ir.Func             { return fe.fn }
func (fe *callsFrontend) Syslook(n string) *obj.LSym { return fe.link.Lookup("runtime." + n) }
func (fe *callsFrontend) UseWriteBarrier() bool      { return fe.wb }
func callsFunc(c *Context, stack bool) (*Func, *callsFrontend) {
	cfg := c.NewConfig(c.IR.Base.Ctxt.Arch.Name, *c.NewTypes(), c.IR.Base.Ctxt, true, false)
	pos := cfg.ctxt.PosTable.XPos(src.MakePos(src.NewFileBase("calls.go", "calls.go"), 10, 1))
	fn := c.IR.NewFunc(pos, pos, c.Types.LocalPkg.Lookup("fixture"), c.Types.NewSignature(nil, nil, nil))
	fn.DeclareParams(true)
	fe := &callsFrontend{genericFrontend: &genericFrontend{diagnosticFrontend: &diagnosticFrontend{}}, fn: fn, link: cfg.ctxt, wb: true}
	cache := new(Cache)
	cache.Reset()
	f := cfg.NewFunc(fe, cache)
	f.Name = "fixture"
	f.Type = fn.Type()
	f.ABI0 = cfg.ABI0
	f.ABI1 = cfg.ABI1
	f.ABISelf = cfg.ABI1
	if stack {
		f.ABISelf = cfg.ABI0
	}
	f.ABIDefault = f.ABISelf
	f.OwnAux = OwnAuxCall(fn.Linksym(), f.ABISelf.ABIAnalyze(fn.Type(), true))
	f.pass = &pass{name: "calls"}
	f.Entry = f.NewBlock(BlockRet)
	f.Entry.Pos = pos
	return f, fe
}
func TestNativeSSATemporaryOwnership(t *testing.T) {
	c, other := loweringContext("amd64"), loweringContext("amd64")
	f, fe := callsFunc(c, false)
	_, foreign := callsFunc(other, false)
	check := func() {
		n := f.NewLocal(f.Entry.Pos, f.Config.Types.Int64)
		if ir.ContextOf(n) != c.IR || n.Curfn != fe.fn || n.Type() != f.Config.Types.Int64 || n.Class != ir.PAUTO || !n.NonMergeable() || !n.AutoTemp() || n.Esc() != ir.EscNever {
			t.Fatalf("invalid temporary: %v", n)
		}
		if fe.fn.Dcl[len(fe.fn.Dcl)-1] != n {
			t.Fatal("temporary not declared")
		}
	}
	check()
	home := fe.fn
	for _, mode := range []string{"function", "type"} {
		t.Run(mode, func(t *testing.T) {
			beforeHome, beforeForeign := len(home.Dcl), len(foreign.fn.Dcl)
			typ := f.Config.Types.Int64
			if mode == "function" {
				fe.fn = foreign.fn
			} else {
				typ = other.Types.Types[types.TINT64]
			}
			func() {
				defer func() {
					if recover() == nil {
						t.Error("foreign owner accepted")
					}
				}()
				f.NewLocal(f.Entry.Pos, typ)
			}()
			fe.fn = home
			if len(home.Dcl) != beforeHome || len(foreign.fn.Dcl) != beforeForeign {
				t.Fatal("rejection mutated declarations")
			}
			check()
		})
	}
}

// Compare expanded call register lanes and stack bytes with the original scalar
// values. The ABI layout is already checked independently by the ABI suite.
func TestNativeCallArgumentBytes(t *testing.T) {
	for _, arch := range []string{"386", "amd64", "mips", "ppc64", "arm64"} {
		for _, stack := range []bool{false, true} {
			t.Run(fmt.Sprint(arch, "/", stack), func(t *testing.T) {
				c := loweringContext(arch)
				f, _ := callsFunc(c, stack)
				b := f.Entry
				pos := b.Pos
				ty := f.Config.Types
				mem := b.NewValue0(pos, OpInitMem, c.Types.TypeMem)
				f.spSb()
				var ins []*types.Type
				var values []*Value
				var numbers []uint64
				for i := 0; i < 24; i++ {
					n := uint64(0xf123456789abcdef) ^ uint64(i)*0x123456789ab
					ins = append(ins, ty.UInt64)
					numbers = append(numbers, n)
					values = append(values, f.ConstInt64(ty.UInt64, int64(n)))
				}
				info := f.ABIDefault.ABIAnalyzeTypes(ins, nil)
				aux := StaticAuxCall(c.IR.Base.Ctxt.Lookup("callee"), info)
				call := b.NewValue0A(pos, OpStaticLECall, aux.LateExpansionResultType(), aux)
				call.AddArgs(values...)
				call.AddArg(mem)
				b.SetControl(b.NewValue1I(pos, OpSelectN, c.Types.TypeMem, 0, call))
				numberLines(f)
				expandCalls(f)
				postExpandCallsDecompose(f)
				opt(f)
				deadcode(f)
				checkFunc(f)
				data := map[int64]byte{}
				var addr func(*Value) int64
				addr = func(v *Value) int64 {
					switch v.Op {
					case OpSP:
						return 0
					case OpOffPtr:
						return addr(v.Args[0]) + v.AuxInt
					case OpCopy:
						return addr(v.Args[0])
					default:
						t.Fatalf("unexpected address %v", v.LongString())
						return 0
					}
				}
				var write func(*Value)
				write = func(v *Value) {
					switch v.Op {
					case OpInitMem:
						return
					case OpCopy:
						write(v.Args[0])
					case OpStore:
						write(v.Args[2])
						n := evalGenericInteger(t, v.Args[1], nil, 0)
						size := int(v.Args[1].Type.Size())
						var raw [8]byte
						if f.Config.BigEndian {
							for i := 0; i < size; i++ {
								raw[size-1-i] = byte(n >> uint(i*8))
							}
						} else {
							for i := 0; i < size; i++ {
								raw[i] = byte(n >> uint(i*8))
							}
						}
						off := addr(v.Args[0])
						for i := 0; i < size; i++ {
							if _, ok := data[off+int64(i)]; ok {
								t.Fatal("overlapping argument stores")
							}
							data[off+int64(i)] = raw[i]
						}
					default:
						t.Fatalf("unexpected memory %v", v.LongString())
					}
				}
				write(call.MemoryArg())
				lane := 0
				expectedBytes := 0
				for i, p := range info.InParams() {
					if len(p.Registers) > 0 {
						if len(p.Registers) != 1 {
							t.Fatal("unexpected register width")
						}
						got := evalGenericInteger(t, call.Args[lane], nil, 0)
						if got != numbers[i] {
							t.Fatalf("register lane %d got %x want %x", lane, got, numbers[i])
						}
						lane++
					} else {
						off := int64(p.Offset())
						for j := 0; j < 8; j++ {
							shift := j
							if f.Config.BigEndian {
								shift = 7 - j
							}
							want := byte(numbers[i] >> uint(shift*8))
							if got, ok := data[off+int64(j)]; !ok || got != want {
								t.Fatalf("arg %d byte %d got %x present %v want %x", i, j, got, ok, want)
							}
							expectedBytes++
						}
					}
				}
				if lane != len(call.Args)-1 || len(data) != expectedBytes {
					t.Fatalf("extra lanes/bytes %d/%d", lane, len(data))
				}
				lower(f)
				lateLower(f)
				deadcode(f)
				checkFunc(f)
				checkLower(f)
			})
		}
	}
}
func TestNativeCallsCompilationIsolation(t *testing.T) {
	old := loweringContext("386")
	f, fe := callsFunc(old, true)
	f.NewLocal(f.Entry.Pos, f.Config.Types.Int64)
	before := len(fe.fn.Dcl)
	var wg sync.WaitGroup
	for i := range 8 {
		wg.Go(func() {
			c := loweringContext([]string{"amd64", "mips", "ppc64", "arm64"}[i%4])
			c.Types.Experiment.CgoCheck2 = i%2 == 0
			f, fe := callsFunc(c, i%2 == 0)
			for range 8 {
				f.NewLocal(f.Entry.Pos, f.Config.Types.BytePtr)
			}
			if len(fe.fn.Dcl) != 8 {
				t.Error("temporary state leaked")
			}
		})
	}
	wg.Wait()
	if len(fe.fn.Dcl) != before || old.Types.Experiment.CgoCheck2 {
		t.Fatal("retained compiler changed")
	}
	f.NewLocal(f.Entry.Pos, f.Config.Types.Int64)
}

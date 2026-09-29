package reflectdata

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"go-backend-ssa/internal/cmd/obj"
	"go-backend-ssa/internal/cmd/src"
	"go-backend-ssa/internal/ir"
	"go-backend-ssa/internal/types"
	"strings"
	"sync"
	"testing"
)

func algorithmContext(size int, merge bool) *Context {
	c := testContext(size, false)
	c.Runtime.Writer.Link.Arch.CanMergeLoads = merge
	c.Typecheck.InitRuntime()
	return c
}
func TestNativeAlgorithmClosuresAndBodies(t *testing.T) {
	for _, size := range []int{4, 8} {
		for _, merge := range []bool{false, true} {
			c := algorithmContext(size, merge)
			u := c.Typecheck.IR.Types
			typesToTest := []*types.Type{u.Types[types.TINT], u.Types[types.TSTRING], u.NewInterface(nil), u.NewArray(u.Types[types.TUINT8], 3), u.NewArray(u.Types[types.TSTRING], 64), u.NewArray(u.Types[types.TFLOAT32], 19)}
			for _, typ := range typesToTest {
				u.CalcSize(typ)
				hash, eq := c.genhash(typ), c.geneq(typ)
				before := len(c.Typecheck.Target.Funcs)
				if c.genhash(typ) != hash || c.geneq(typ) != eq || len(c.Typecheck.Target.Funcs) != before {
					t.Fatal("algorithm cache identity")
				}
				for _, closure := range []*obj.LSym{hash, eq} {
					if closure == nil || len(closure.P) < size || len(closure.R) != 1 || closure.R[0].Siz != uint8(size) || closure.R[0].Sym.ABI() != obj.ABIInternal || !closure.OnList() {
						t.Fatal("native closure and function relocation", closure)
					}
				}
			}
			// A three-byte regular region uses a runtime varlen closure with captured length.
			typ := u.NewArray(u.Types[types.TUINT8], 3)
			u.CalcSize(typ)
			for _, sym := range []*obj.LSym{c.genhash(typ), c.geneq(typ)} {
				var length uint64
				if size == 4 {
					length = uint64(binary.LittleEndian.Uint32(sym.P[size:]))
				} else {
					length = binary.LittleEndian.Uint64(sym.P[size:])
				}
				if length != 3 || sym.Size != int64(2*size) {
					t.Fatal("varlen closure payload", sym.Name, length)
				}
			}
			if c.memhashvarlen != c.Typecheck.LookupRuntimeFunc("memhash_varlen") || c.memequalvarlen != c.Typecheck.LookupRuntimeFunc("memequal_varlen") {
				t.Fatal("runtime cache binding")
			}
			if len(c.Typecheck.Target.Funcs) == 0 {
				t.Fatal("no native algorithm bodies generated")
			}
			loops, calls := 0, 0
			for _, fn := range c.Typecheck.Target.Funcs {
				if !fn.Dupok() || !fn.NilCheckDisabled() || fn.Pragma&ir.Noinline == 0 || len(fn.Body) == 0 || fn.Nname.Func != fn {
					t.Fatal("native function metadata")
				}
				ir.VisitList(fn.Body, func(n ir.Node) {
					if n.Op() == ir.OFOR {
						loops++
					}
					if n.Op() == ir.OCALLFUNC {
						calls++
					}
					if n.Type() != nil && n.Type().Universe() != u {
						t.Error("foreign type in generated body")
					}
				})
			}
			if loops == 0 || calls == 0 || c.Typecheck.IR.CurFunc != nil {
				t.Fatal("native loop/call construction or function restoration", loops, calls)
			}
			c.Typecheck.CheckFuncStack()
			if c.geneq(u.NewSlice(u.Types[types.TINT])) != nil {
				t.Fatal("non-comparable type has equality closure")
			}
		}
	}
}
func TestIndependentAlgorithmCachesAndTrace(t *testing.T) {
	var wg sync.WaitGroup
	for i := range 12 {
		wg.Go(func() {
			c := algorithmContext(4+4*(i%2), i%3 == 0)
			u := c.Typecheck.IR.Types
			var trace bytes.Buffer
			c.Typecheck.IR.TraceWriter = &trace
			c.Typecheck.IR.Base.Flag.LowerR = 1
			typ := u.NewArray(u.Types[types.TSTRING], 64)
			u.CalcSize(typ)
			eq := c.geneq(typ)
			hash := c.genhash(typ)
			firstBody := c.Typecheck.Target.Funcs[0]
			bodyText := fmt.Sprint(firstBody.Body)
			oldBytes := bytes.Clone(eq.P)
			oldTrace := trace.String()
			if !strings.Contains(oldTrace, "geneq") || !strings.Contains(oldTrace, "genhash") {
				t.Error("native trace not routed")
			}
			other := algorithmContext(12-u.PointerSize(), false)
			ot := other.Typecheck.IR.Types.NewArray(other.Typecheck.IR.Types.Types[types.TSTRING], 64)
			other.Typecheck.IR.Types.CalcSize(ot)
			if other.geneq(ot) == eq || other.genhash(ot) == hash || fmt.Sprint(firstBody.Body) != bodyText || !bytes.Equal(oldBytes, eq.P) || trace.String() != oldTrace {
				t.Error("another compilation modified old algorithms or trace")
			}
			if c.geneq(typ) != eq || c.genhash(typ) != hash {
				t.Error("retained algorithm cache")
			}
		})
	}
	wg.Wait()
}
func TestEqualitySignatureAndPublicSelection(t *testing.T) {
	c := algorithmContext(8, true)
	u := c.Typecheck.IR.Types
	fields := []*types.Field{u.NewField(src.XPos{}, u.LocalPkg.Lookup("A"), u.Types[types.TUINT8]), u.NewField(src.XPos{}, u.LocalPkg.Lookup("_"), u.Types[types.TUINT8]), u.NewField(src.XPos{}, u.LocalPkg.Lookup("S"), u.Types[types.TSTRING])}
	record := u.NewStruct(fields)
	u.CalcSize(record)
	if sig := c.eqSignature(record); sig != "M1K7S" {
		t.Fatal("padding/blank signature", sig)
	}
	fn, needsLen := c.EqFor(record)
	if needsLen || fn.(*ir.Name).Func == nil {
		t.Fatal("special equality function")
	}
	mem := u.NewArray(u.Types[types.TINT], 3)
	u.CalcSize(mem)
	generic, needsLen := c.EqFor(mem)
	if !needsLen || generic != c.Typecheck.LookupRuntime("memequal") {
		t.Fatal("memory equality selection")
	}
	other := algorithmContext(4, false)
	before := len(c.Typecheck.Target.Funcs)
	for _, call := range []func(){func() { c.EqFor(other.Typecheck.IR.Types.Types[types.TINT]) }, func() { c.genhash(nil) }, func() { c.parseNum("bad") }, func() { c.parseArray("2S") }} {
		mustPanic(t, call)
	}
	if len(c.Typecheck.Target.Funcs) != before {
		t.Fatal("invalid request declared a function")
	}
}

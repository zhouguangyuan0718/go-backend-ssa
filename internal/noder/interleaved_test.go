package noder

import (
	"bytes"
	"fmt"
	"go-backend-ssa/internal/cmd/pgo"
	"go-backend-ssa/internal/devirtualize"
	"go-backend-ssa/internal/ir"
	"strings"
	"testing"
)

const interleavedSource = `package p
var Sink int
func Add(x int)int{return x+1}
type I interface {M(int)int}
type A int
func(a A)M(x int)int{return Add(int(a)+x)}
type B int
func(b B)M(x int)int{return int(b)-x}
func MakeI(x int)I{return A(x)}
func Direct(x int)int{var i I=A(x);return i.M(1)}
func Factory(x int)int{i:=MakeI(x);return i.M(2)}
func Same(x int,b bool)int{var i I;if b{i=A(x)}else{i=A(x+1)};return i.M(3)}
func Mixed(x int,b bool)int{var i I=A(x);if b{i=B(x)};return i.M(4)}
func Unknown(i I)int{return i.M(5)}
func Late(x int)int{var i I;r:=i.M(6);i=A(x);return r}
func Alias(x int)int{var i I=A(x);j:=i;i=j;return j.M(7)}
func Nil()int{var i I;return i.M(8)}
func Deferred(x int){var i I=A(x);defer i.M(9)}
func GoCall(x int){var i I=A(x);go i.M(10)}
func Slice(xs []A)int{r:=0;for _,a:=range xs{var i I=a;r+=i.M(1)};return r}
func Map(xs map[int]A)int{r:=0;for _,a:=range xs{var i I=a;r+=i.M(1)};return r}
func Chan(xs chan A)int{r:=0;for a:=range xs{var i I=a;r+=i.M(1)};return r}
func Maker(x int)func()int{Sink++;return func()int{return x}}
func Nested(x int)int{return Maker(x)()}
func ClosureTwice(x int)int{f:=func()int{return x};return f()+f()}
func Pair(x int)(I,int){return A(x),x}
func Multi(x int)int{i,n:=Pair(x);return i.M(n)}
func Rec(x int)int{if x==0{return 0};return Rec(x-1)}
func Indirect(f func(int)int,x int)int{return f(x)}
`

func interleavedContext(t *testing.T, newInliner bool) *Context {
	t.Helper()
	c := bodyContext(t)
	c.Types.Experiment.NewInliner = newInliner
	c.Flag.LowerL = 1
	c.Debug.InlFuncsWithClosures = 1
	c.ReadLocalPackage(localStub(t, c, interleavedSource))
	return c
}
func driverFunc(t *testing.T, c *Context, name string) *ir.Func {
	t.Helper()
	fn, err := c.LookupFunc("example/p." + name)
	if err != nil {
		t.Fatal(err)
	}
	return fn
}
func nodeCounts(fn *ir.Func) map[ir.Op]int {
	counts := map[ir.Op]int{}
	ir.VisitList(fn.Body, func(n ir.Node) { counts[n.Op()]++ })
	return counts
}
func TestNativeInterleavedPackage(t *testing.T) {
	for _, newInliner := range []bool{false, true} {
		t.Run(fmt.Sprint(newInliner), func(t *testing.T) {
			c := interleavedContext(t, newInliner)
			var dump bytes.Buffer
			c.Typecheck.IR.TraceWriter = &dump
			c.Debug.DumpInlFuncProps = "host-stream"
			c.Interleaved.DevirtualizeAndInlinePackage(c.Typecheck.Target, nil)
			if c.Typecheck.IR.CurFunc != nil {
				t.Fatal("current function leaked")
			}
			for _, fn := range c.Typecheck.Target.Funcs {
				if nodeCounts(fn)[ir.OPAREN] != 0 {
					t.Fatal("driver left parentheses", fn)
				}
			}
			for _, name := range []string{"Direct", "Factory", "Same", "Alias", "Slice", "Map", "Chan", "Nested", "ClosureTwice", "Multi"} {
				n := nodeCounts(driverFunc(t, c, name))
				if n[ir.OCALLINTER] != 0 || n[ir.OCALLFUNC] != 0 || n[ir.OINLCALL] == 0 {
					t.Fatal(name, n)
				}
			}
			for _, name := range []string{"Mixed", "Unknown", "Nil"} {
				if nodeCounts(driverFunc(t, c, name))[ir.OCALLINTER] != 1 {
					t.Fatal("unsafe interface rewrite", name)
				}
			}
			// The reader normalizes go/defer into a wrapper. Its body may
			// optimize, but the scheduled call and outer nil check must remain.
			for _, name := range []string{"Deferred", "GoCall"} {
				count := 0
				ir.VisitList(driverFunc(t, c, name).Body, func(n ir.Node) {
					if call, ok := n.(*ir.CallExpr); ok && call.GoDefer {
						count++
					}
				})
				if count != 1 || nodeCounts(driverFunc(t, c, name))[ir.OCHECKNIL] != 1 {
					t.Fatal("scheduled call or nil check lost", name, count)
				}
			}
			if nodeCounts(driverFunc(t, c, "Nested"))[ir.OASOP] != 1 {
				t.Fatal("nested callee effects duplicated")
			}
			if nodeCounts(driverFunc(t, c, "Rec"))[ir.OCALLFUNC] != 1 {
				t.Fatal("recursive driver did not stop")
			}
			if !strings.Contains(dump.String(), "<endfilepreamble>") || !strings.Contains(dump.String(), "<endfuncpreamble>") {
				t.Fatal("missing function properties output")
			}
		})
	}
}
func TestNativeInterleavedSingleFunc(t *testing.T) {
	for _, newInliner := range []bool{false, true} {
		t.Run(fmt.Sprint(newInliner), func(t *testing.T) {
			c := interleavedContext(t, newInliner)
			c.Inline.CanInlineFuncs(c.Typecheck.Target.Funcs, nil)
			fn := driverFunc(t, c, "Factory")
			c.Interleaved.DevirtualizeAndInlineFunc(fn, nil)
			n := nodeCounts(fn)
			if n[ir.OCALLINTER] != 0 || n[ir.OCALLFUNC] != 0 || n[ir.OPAREN] != 0 || n[ir.OINLCALL] < 2 {
				t.Fatal(n)
			}
		})
	}
}
func TestNativeStaticDevirtualize(t *testing.T) {
	c := interleavedContext(t, false)
	c.Flag.LowerL = 0
	c.Interleaved.DevirtualizeAndInlinePackage(c.Typecheck.Target, nil)
	for _, name := range []string{"Direct", "Same", "Alias", "Late", "Slice", "Map", "Chan"} {
		fn := driverFunc(t, c, name)
		n := nodeCounts(fn)
		if n[ir.OCALLINTER] != 0 || n[ir.OCALLFUNC] != 1 || n[ir.OINLCALL] != 0 {
			t.Fatal(name, n)
		}
		var safe bool
		ir.VisitList(fn.Body, func(n ir.Node) {
			if a, ok := n.(*ir.TypeAssertExpr); ok {
				safe = a.UseNilPanic
			}
		})
		if !safe {
			t.Fatal("nil interface panic semantics lost", name)
		}
	}
	if nodeCounts(driverFunc(t, c, "Factory"))[ir.OCALLINTER] != 1 {
		t.Fatal("non-inlined result must remain indirect")
	}
}
func TestNativeInterleavedIsolation(t *testing.T) {
	old := interleavedContext(t, false)
	old.Interleaved.DevirtualizeAndInlinePackage(old.Typecheck.Target, nil)
	fn := driverFunc(t, old, "Factory")
	before := fmt.Sprint(nodeCounts(fn))
	typ := fn.Type()
	for i := 0; i < 8; i++ {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			t.Parallel()
			c := interleavedContext(t, i%2 == 0)
			c.Interleaved.DevirtualizeAndInlinePackage(c.Typecheck.Target, nil)
			if c.Types == old.Types || c.Ctxt == old.Ctxt || fmt.Sprint(nodeCounts(fn)) != before || fn.Type() != typ {
				t.Fatal("retained graph changed")
			}
		})
	}
}
func TestNativeDevirtualizeOwnership(t *testing.T) {
	a, b := interleavedContext(t, false), interleavedContext(t, false)
	call := func(c *Context) *ir.CallExpr {
		var out *ir.CallExpr
		ir.VisitList(driverFunc(t, c, "Unknown").Body, func(n ir.Node) {
			if v, ok := n.(*ir.CallExpr); ok {
				out = v
			}
		})
		return out
	}
	ac, bc := call(a), call(b)
	var state devirtualize.State
	reject := func(f func()) {
		t.Helper()
		defer func() {
			if recover() == nil {
				t.Error("foreign input accepted")
			}
		}()
		f()
	}
	reject(func() { a.Interleaved.Devirtualize.StaticCall(&state, bc) })
	a.Interleaved.Devirtualize.StaticCall(&state, ac)
	reject(func() { b.Interleaved.Devirtualize.StaticCall(&state, bc) })
	reject(func() {
		state.InlinedCall(driverFunc(t, b, "Unknown"), bc, b.Typecheck.IR.NewInlinedCallExpr(bc.Pos(), nil, nil))
	})
	reject(func() { a.Interleaved.DevirtualizeAndInlinePackage(b.Typecheck.Target, nil) })
	reject(func() { a.Interleaved.DevirtualizeAndInlineFunc(driverFunc(t, b, "Unknown"), nil) })
	if ac.Op() != ir.OCALLINTER || bc.Op() != ir.OCALLINTER {
		t.Fatal("rejected input mutated")
	}
}
func TestNativePGODevirtualize(t *testing.T) {
	for _, name := range []string{"Unknown", "Indirect"} {
		t.Run(name, func(t *testing.T) {
			c := interleavedContext(t, false)
			c.Debug.PGODevirtualize = 2
			c.Debug.PGOInline = 1
			c.Debug.PGODebug = 3
			target := "example/p.A.M"
			if name == "Indirect" {
				target = "example/p.Add"
			}
			input, err := pgo.FromSerialized(strings.NewReader("GO PREPROFILE V1\nexample/p." + name + "\n" + target + "\n0 100\n"))
			if err != nil {
				t.Fatal(err)
			}
			profile := c.Inline.PGO.NewProfile(input)
			var trace bytes.Buffer
			c.Typecheck.IR.TraceWriter = &trace
			c.Interleaved.DevirtualizeAndInlinePackage(c.Typecheck.Target, profile)
			n := nodeCounts(driverFunc(t, c, name))
			if n[ir.OIF] != 1 || n[ir.OINLCALL] < 2 {
				t.Fatal("profile did not specialize call", name, n, trace.String())
			}
			if name == "Unknown" && n[ir.OCALLINTER] != 1 {
				t.Fatal("interface fallback lost")
			}
			if name == "Indirect" && n[ir.OCALLFUNC] != 1 {
				t.Fatal("function fallback lost")
			}
			if !strings.Contains(trace.String(), `"Devirtualized":`) {
				t.Fatal("PGO JSON bypassed instance trace")
			}
			other := interleavedContext(t, false)
			defer func() {
				if recover() == nil {
					t.Error("foreign profile accepted")
				}
			}()
			other.Interleaved.DevirtualizeAndInlinePackage(other.Typecheck.Target, profile)
		})
	}
}

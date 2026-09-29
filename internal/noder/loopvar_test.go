package noder

import (
	"bytes"
	"fmt"
	"go-backend-ssa/internal/ir"
	"go-backend-ssa/internal/logopt"
	"go-backend-ssa/internal/loopvar"
	"strings"
	"testing"
)

const loopvarSource = `package p
func RangeAddr(xs []int)(out []*int){for _,v:=range xs{out=append(out,&v)};return}
func RangeBoth(xs []int)(out []func()int){for k,v:=range xs{out=append(out,func()int{return k+v})};return}
func ForCapture(n int)(out []func()int){for i:=0;i<n;i++{out=append(out,func()int{return i})};return}
func Continue(n int)(out []*int){outer:for i:=0;i<n;i++{out=append(out,&i);for j:=0;j<2;j++{if j==1{continue outer};continue};if i==2{continue}};return}
func NoPost(n int)(out []*int){for i:=0;i<n;{out=append(out,&i);i++};return}
func NoCond(n int)(out []*int){for i:=0;;i++{if i==n{break};out=append(out,&i)};return}
func Multi(n int)(out []*int){for i,j:=0,1;i<n;i,j=i+1,j+2{out=append(out,&i,&j)};return}
func Nested(n int)(out []*int){for i:=0;i<n;i++{for j:=0;j<n;j++{out=append(out,&i,&j)}};return}
func MapRange(xs map[int]int)(out []*int){for k,v:=range xs{out=append(out,&k,&v)};return}
func ChanRange(xs chan int)(out []*int){for v:=range xs{out=append(out,&v)};return}
func StringRange(s string)(out []func()int){for k,v:=range s{out=append(out,func()int{return k+int(v)})};return}
func ArrayRange(a *[2]int)(out []*int){for _,v:=range a{out=append(out,&v)};return}
func AssignRange(xs []int)(out []*int){var v int;for _,v=range xs{out=append(out,&v)};return}
func NoLeak(n int)(r int){for i:=0;i<n;i++{r+=i};return}
func ReturnAddr(n int)*int{for i:=0;i<n;i++{if i==2{return &i}};return nil}
func ReturnRange(xs []int)*int{for _,v:=range xs{return &v};return nil}
func ReturnClosure(n int)func()int{for i:=0;i<n;i++{return func()int{return i}};return nil}
func InlineCapture(xs []int)[]*int{return RangeAddr(xs)}
func NestedMaker(n int)func()[]*int{return func()(out []*int){for i:=0;i<n;i++{out=append(out,&i)};return}}
`

func loopvarContext(t *testing.T, newInliner bool) (*Context, *loopvar.Context) {
	t.Helper()
	c := bodyContext(t)
	c.Types.Experiment.NewInliner = newInliner
	c.Flag.LowerL = 1
	c.Debug.InlFuncsWithClosures = 1
	c.ReadLocalPackage(localStub(t, c, loopvarSource))
	c.Interleaved.DevirtualizeAndInlinePackage(c.Typecheck.Target, nil)
	c.MakeWrappers(c.Typecheck.Target)
	return c, loopvar.NewContext(c.Typecheck, logopt.NewContext())
}
func TestNativeLoopvarCapture(t *testing.T) {
	for _, newInliner := range []bool{false, true} {
		t.Run(fmt.Sprint(newInliner), func(t *testing.T) {
			c, lv := loopvarContext(t, newInliner)
			expected := map[string]int{"RangeAddr": 1, "RangeBoth": 2, "ForCapture": 1, "Continue": 1, "NoPost": 1, "NoCond": 1, "Multi": 2, "Nested": 2, "MapRange": 2, "ChanRange": 1, "StringRange": 2, "ArrayRange": 1, "AssignRange": 0, "NoLeak": 0, "ReturnAddr": 0, "ReturnRange": 0, "ReturnClosure": 0, "InlineCapture": 1}
			for _, fn := range c.Typecheck.Target.Funcs {
				records := lv.ForCapture(fn)
				if want, ok := expected[fn.Sym().Name]; ok && len(records) != want {
					t.Fatal(fn, len(records), want)
				}
				for _, r := range records {
					if r.Name.Curfn != fn || !c.Typecheck.IR.OwnsNode(r.Loop) {
						t.Fatal("record lost graph ownership")
					}
					decls := 0
					ir.Visit(fn, func(n ir.Node) {
						if d, ok := n.(*ir.Decl); ok && d.X == r.Name {
							decls++
						}
					})
					if decls != 1 {
						t.Fatal("declaration lost or duplicated", fn, r.Name, decls)
					}
				}
				ir.Visit(fn, func(n ir.Node) {
					switch n := n.(type) {
					case *ir.RangeStmt:
						if n.DistinctVars {
							t.Error("range marker remains")
						}
					case *ir.ForStmt:
						if n.DistinctVars {
							t.Error("for marker remains")
						}
					}
				})
				if again := lv.ForCapture(fn); len(again) != 0 {
					t.Fatal("loop rewritten twice")
				}
			}
			if c.Typecheck.IR.CurFunc != nil {
				t.Fatal("current function leaked")
			}
			fn := driverFunc(t, c, "Continue")
			counts := nodeCounts(fn)
			if counts[ir.OGOTO] != 2 || counts[ir.OCONTINUE] != 1 {
				t.Fatal("continue targets changed", counts)
			}
			fn = driverFunc(t, c, "NoPost")
			if nodeCounts(fn)[ir.OIF] != 1 {
				t.Fatal("unnecessary first-iteration guard")
			}
		})
	}
}
func TestNativeLoopvarHashAndForce(t *testing.T) {
	total := 0
	for _, pattern := range []string{"", "n", "0", "1"} {
		t.Run(pattern, func(t *testing.T) {
			c, lv := loopvarContext(t, false)
			var trace bytes.Buffer
			c.Base.LoopVarHash = c.Base.NewHashDebug("loopvarhash", pattern, &trace)
			count := 0
			for _, fn := range c.Typecheck.Target.Funcs {
				count += len(lv.ForCapture(fn))
			}
			switch pattern {
			case "":
				total = count
			case "n":
				if count != 0 {
					t.Fatal("hash exclusion ignored")
				}
			case "0", "1":
				total -= count
			}
			if pattern == "0" || pattern == "1" {
				if !strings.Contains(trace.String(), "loopvarhash triggered") {
					t.Fatal("hash trace missing")
				}
			}
		})
	}
	if total != 0 {
		t.Fatal("complementary hash selections do not cover baseline", total)
	}
	c, lv := loopvarContext(t, false)
	c.Debug.LoopVar = 11
	if len(lv.ForCapture(driverFunc(t, c, "NoLeak"))) != 1 {
		t.Fatal("forced transform ignored")
	}
}
func TestNativeLoopvarDiagnostics(t *testing.T) {
	c, lv := loopvarContext(t, false)
	c.Debug.LoopVar = 4
	lv.Log.Format = logopt.Json0
	records := lv.ForCapture(driverFunc(t, c, "InlineCapture"))
	if len(records) != 1 {
		t.Fatal(records)
	}
	// Exercise reporting inputs explicitly; this test does not implement escape analysis.
	records[0].Name.SetEsc(ir.EscHeap)
	lv.LogTransformations(records)
	events := lv.Log.Snapshot()
	if len(events) != 2 || events[0].What() != "iteration-variable-to-heap" || events[1].What() != "loop-modified-range" {
		t.Fatal("native events missing", events)
	}
	ds := c.Diagnostics.Diagnostics()
	if len(ds) != 2 || !strings.Contains(ds[0].Message, "loop inlined into") || !strings.Contains(ds[0].Message, "heap-allocated") {
		t.Fatal("inline diagnostics lost", ds)
	}
	records[0].Name.SetEsc(ir.EscNone)
	c.Debug.LoopVar = 11
	lv.LogTransformations(records)
	if len(c.Diagnostics.Diagnostics()) != 2 || lv.Log.Snapshot()[2].What() != "iteration-variable-to-stack" {
		t.Fatal("quiet logging or stack classification lost")
	}
}
func TestNativeLoopvarIsolation(t *testing.T) {
	old, lv := loopvarContext(t, false)
	fn := driverFunc(t, old, "RangeAddr")
	kept := lv.ForCapture(fn)
	before := fmt.Sprint(nodeCounts(fn))
	name := kept[0].Name
	for i := 0; i < 8; i++ {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			t.Parallel()
			c, lv := loopvarContext(t, i%2 != 0)
			lv.Log.Format = logopt.Json0
			for _, fn := range c.Typecheck.Target.Funcs {
				lv.ForCapture(fn)
			}
			if kept[0].Name != name || name.Curfn != fn || fmt.Sprint(nodeCounts(fn)) != before {
				t.Fatal("retained capture graph changed")
			}
			defer func() {
				if recover() == nil {
					t.Error("foreign record accepted")
				}
				if len(lv.Log.Snapshot()) != 0 {
					t.Error("foreign record emitted log")
				}
			}()
			lv.LogTransformations(kept)
		})
	}
}
func TestNativeLoopvarForeignFunction(t *testing.T) {
	a, lv := loopvarContext(t, false)
	b, _ := loopvarContext(t, false)
	fn := driverFunc(t, b, "RangeAddr")
	before := fmt.Sprint(nodeCounts(fn))
	defer func() {
		if recover() == nil {
			t.Error("foreign function accepted")
		}
		if fmt.Sprint(nodeCounts(fn)) != before || a.Typecheck.IR.CurFunc != nil {
			t.Error("foreign rewrite mutated state")
		}
	}()
	lv.ForCapture(fn)
}

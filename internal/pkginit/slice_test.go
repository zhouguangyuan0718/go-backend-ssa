package pkginit

import (
	"bytes"
	"fmt"
	"go-backend-ssa/internal/deadlocals"
	"go-backend-ssa/internal/escape"
	"go-backend-ssa/internal/ir"
	"go-backend-ssa/internal/slice"
	"go-backend-ssa/internal/ssagen"
	"strings"
	"testing"
)

const sliceSource = `package p
var Global []int
func Grow(n int)[]int{var s []int;for i:=0;i<n;i++{s=append(s,i)};return s}
func Capacity(n int)([]int,int){var s []int;for i:=0;i<n;i++{s=append(s,i)};c:=cap(s);return s,c}
func Named(n int)(s []int){for i:=0;i<n;i++{s=append(s,i)};return}
func Assign(n int,p *[]int){var s []int;for i:=0;i<n;i++{s=append(s,i)};*p=s}
func Range(n int)int{var s []int;for i:=0;i<n;i++{s=append(s,i)};sum:=0;for _,v:=range s{sum+=v};return sum}
func Aliased(n int)([]int,[]int){var s,alias []int;for i:=0;i<n;i++{s=append(s,i);alias=s};return s,alias}
func Few(n int)[]int{var s []int;s=append(s,n);return s}
func Local(n int)int{var s []int;for i:=0;i<n;i++{s=append(s,i)};return len(s)}
func Read(s []int)int{return len(s)}
func Retain(s []int){Global=s}
func ReturnAlias(s []int)[]int{return s}
func CallRead(n int)[]int{var s []int;for i:=0;i<n;i++{s=append(s,i)};Read(s);return s}
func CallRetain(n int)[]int{var s []int;for i:=0;i<n;i++{s=append(s,i)};Retain(s);return s}
func CallReturn(n int)[]int{var s []int;for i:=0;i<n;i++{s=append(s,i)};ReturnAlias(s);return s}
func Indirect(n int,f func([]int))[]int{var s []int;for i:=0;i<n;i++{s=append(s,i)};f(s);return s}
func Address(n int)([]int,*int){var s []int;for i:=0;i<n;i++{s=append(s,i)};p:=&s[0];return s,p}
func Multiple(n int,b bool)[]int{var s []int;for i:=0;i<n;i++{s=append(s,i)};if b{return s};return s}
func Large(n int)[][65]byte{var s [][65]byte;for i:=0;i<n;i++{s=append(s,[65]byte{})};return s}
func Zero(n int)[]struct{}{var s []struct{};for i:=0;i<n;i++{s=append(s,struct{}{})};return s}
type Slice []int
func(s Slice)Len()int{return len(s)}
func Method(n int)Slice{var s Slice;for i:=0;i<n;i++{s=append(s,i)};s.Len();return s}
`

func sliceContext(t *testing.T) *slice.Context {
	t.Helper()
	p := sourceContext(t, "example/p", sliceSource, false)
	p.MakeTask()
	tc := p.Noder.Typecheck
	ssagen.NewContext(tc).NewSymABIs().GenABIWrappers()
	deadlocals.NewContext(tc.IR).Funcs(tc.Target.Funcs)
	esc := escape.NewContext(tc, p.Noder.Inline.Log)
	esc.Funcs(tc.Target.Funcs)
	tc.IR.Debug.VariableMakeThreshold = 64
	return slice.NewContext(esc, p.Noder.ReflectData)
}
func sliceMoves(c *slice.Context, name string) []*ir.MoveToHeapExpr {
	var moves []*ir.MoveToHeapExpr
	ir.Visit(escapeFunc(c.Escape, name), func(n ir.Node) {
		if m, ok := n.(*ir.MoveToHeapExpr); ok {
			moves = append(moves, m)
		}
	})
	return moves
}
func TestNativeSlice(t *testing.T) {
	c := sliceContext(t)
	tc := c.Escape.Typecheck
	c.Funcs(tc.Target.Funcs)
	for _, name := range []string{"Grow", "Capacity", "Named", "Assign", "Range", "CallRead", "Zero", "Method"} {
		moves := sliceMoves(c, name)
		if len(moves) != 1 {
			t.Errorf("%s: got %d moves", name, len(moves))
			continue
		}
		m := moves[0]
		capacity := name == "Capacity" || name == "CallRead" || name == "Method"
		if m.PreserveCapacity != capacity || m.Type() != m.Slice.Type() || m.Typecheck() != 1 || m.RType == nil || !tc.IR.OwnsNode(m) || !tc.IR.OwnsNode(m.RType) {
			t.Errorf("%s: native move metadata", name)
		}
		ir.Visit(escapeFunc(c.Escape, name), func(n ir.Node) {
			if a, ok := n.(*ir.CallExpr); ok && a.Op() == ir.OAPPEND {
				if a.Esc() != ir.EscNone || a.UseBuf != capacity {
					t.Errorf("%s: append state esc=%d buffer=%t", name, a.Esc(), a.UseBuf)
				}
			}
		})
	}
	for _, name := range []string{"Aliased", "Few", "Local", "CallRetain", "CallReturn", "Indirect", "Address", "Multiple", "Large"} {
		if len(sliceMoves(c, name)) != 0 {
			t.Errorf("%s: unsafe or unprofitable move", name)
		}
	}
	for _, fn := range tc.Target.Funcs {
		for _, n := range fn.Dcl {
			if n.Opt != nil {
				t.Errorf("%v retains analysis state", n)
			}
		}
	}
}
func TestNativeSliceDisabled(t *testing.T) {
	for _, mode := range []string{"N", "hash"} {
		t.Run(mode, func(t *testing.T) {
			c := sliceContext(t)
			before := escapeDump(c.Escape)
			if mode == "N" {
				c.Escape.Typecheck.IR.Base.Flag.N = 1
			} else {
				b := c.Escape.Typecheck.IR.Base
				b.VariableMakeHash = b.NewHashDebug("variablemakehash", "n", nil)
			}
			c.Funcs(c.Escape.Typecheck.Target.Funcs)
			if escapeDump(c.Escape) != before {
				t.Fatal("disabled pass changed IR")
			}
		})
	}
	c := sliceContext(t)
	c.Escape.Typecheck.IR.Debug.VariableMakeThreshold = 7
	c.Funcs(c.Escape.Typecheck.Target.Funcs)
	if len(sliceMoves(c, "Grow")) != 0 || len(sliceMoves(c, "Zero")) != 1 {
		t.Fatal("element-size threshold")
	}
}
func TestNativeSliceIsolation(t *testing.T) {
	old := sliceContext(t)
	old.Funcs(old.Escape.Typecheck.Target.Funcs)
	before := escapeDump(old.Escape)
	for i := 0; i < 8; i++ {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			t.Parallel()
			c := sliceContext(t)
			var trace bytes.Buffer
			c.Escape.Typecheck.IR.TraceWriter = &trace
			c.Escape.Typecheck.IR.Debug.AstDump = "Grow"
			reportText := capturePhaseReports(t, c.Escape.Typecheck.IR)
			c.Funcs(c.Escape.Typecheck.Target.Funcs)
			if escapeDump(old.Escape) != before || len(sliceMoves(c, "Grow")) != 1 || !strings.Contains(reportText(), "slice, Grow") {
				t.Fatal("retained graph or trace isolation")
			}
		})
	}
}
func TestNativeSliceRejectsForeignFuncs(t *testing.T) {
	a, b := sliceContext(t), sliceContext(t)
	before := escapeDump(a.Escape)
	defer func() {
		if recover() == nil {
			t.Error("foreign function accepted")
		}
		if before != escapeDump(a.Escape) {
			t.Error("mutated graph before rejection")
		}
	}()
	a.Funcs(append(append([]*ir.Func(nil), a.Escape.Typecheck.Target.Funcs...), b.Escape.Typecheck.Target.Funcs[0]))
}
func TestNativeSliceRejectsForeignDependencies(t *testing.T) {
	a, b := sliceContext(t), sliceContext(t)
	defer func() {
		if recover() == nil {
			t.Error("mixed compilation dependencies accepted")
		}
	}()
	slice.NewContext(a.Escape, b.ReflectData)
}

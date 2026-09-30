package pkginit

import (
	"bytes"
	"fmt"
	"go-backend-ssa/internal/deadlocals"
	"go-backend-ssa/internal/escape"
	"go-backend-ssa/internal/ir"
	"go-backend-ssa/internal/noder"
	"go-backend-ssa/internal/ssagen"
	"strings"
	"testing"
)

const escapeSource = `package p
var Global *int
func Heap()*int{x:=1;return &x}
func InlineHeap()*int{return Heap()}
func Stack(p int)int{x:=p;p2:=&x;return *p2}
func Identity(p *int)*int{return p}
func Store(p *int){Global=p}
func ByValue(x int)func()int{return func()int{return x}}
func ByReference(x int)func()int{return func()int{x++;return x}}
func CycleA(p *int,n int)*int{if n==0{return p};return CycleB(p,n-1)}
func CycleB(p *int,n int)*int{return CycleA(p,n)}
func SliceLength()[]int{n:=4;return make([]int,n)}
func ReadOnly(s string)int{b:=[]byte(s);return len(b)}
func Writable(s string)byte{b:=[]byte(s);b[0]=1;return b[0]}
func Deferred(p *int){defer func(){*p++}()}
func Async(p *int){go func(){*p++}()}
func Aggregate(p *int)*struct{P *int}{return &struct{P *int}{p}}
func AppendLocal(n int)int{s:=make([]int,0,4);for i:=0;i<n;i++{s=append(s,i)};return len(s)}
func AppendAliased(n int)int{s:=make([]int,0,4);var alias []int;for i:=0;i<n;i++{s=append(s,i);alias=s};return len(alias)}
`

func escapeContext(t *testing.T) *escape.Context {
	t.Helper()
	p := sourceContext(t, "example/p", escapeSource, false)
	p.MakeTask()
	tc := p.Noder.Typecheck
	ssagen.NewContext(tc).NewSymABIs().GenABIWrappers()
	deadlocals.NewContext(tc.IR).Funcs(tc.Target.Funcs)
	return escape.NewContext(tc, p.Noder.Inline.Log)
}
func escapeFunc(c *escape.Context, name string) *ir.Func {
	for _, fn := range c.Typecheck.Target.Funcs {
		if fn.Sym().Name == name {
			return fn
		}
	}
	panic("missing function " + name)
}
func escapeLocal(c *escape.Context, fn, name string) *ir.Name {
	for _, n := range escapeFunc(c, fn).Dcl {
		if n.Sym().Name == name {
			return n
		}
	}
	panic("missing local " + fn + "." + name)
}
func escapeDump(c *escape.Context) string {
	var b bytes.Buffer
	for _, fn := range c.Typecheck.Target.Funcs {
		ir.FDump(&b, "snapshot", fn)
	}
	return b.String()
}
func TestNativeEscape(t *testing.T) {
	c := escapeContext(t)
	c.Typecheck.IR.Debug.ZeroCopy = 1
	c.Funcs(c.Typecheck.Target.Funcs)
	for _, test := range []struct {
		fn, name string
		esc      uint16
	}{{"Heap", "x", ir.EscHeap}, {"Stack", "x", ir.EscNone}, {"ByReference", "x", ir.EscHeap}, {"ByValue", "x", ir.EscNone}} {
		n := escapeLocal(c, test.fn, test.name)
		if n.Esc() != test.esc || n.Opt != nil {
			t.Errorf("%s.%s escape=%d opt=%v", test.fn, test.name, n.Esc(), n.Opt)
		}
	}
	for _, name := range []string{"Identity"} {
		summary := c.ParseLeaks(escapeFunc(c, name).Type().Params()[0].Note)
		if summary.Heap() != -1 || summary.Result(0) != 0 {
			t.Errorf("%s return escape tag: heap=%d result=%d note=%q", name, summary.Heap(), summary.Result(0), escapeFunc(c, name).Type().Params()[0].Note)
		}
	}
	for _, name := range []string{"CycleA", "CycleB"} {
		if c.ParseLeaks(escapeFunc(c, name).Type().Params()[0].Note).Heap() != 0 {
			t.Errorf("%s lost native conservative recursive tag", name)
		}
	}
	if c.ParseLeaks(escapeFunc(c, "Store").Type().Params()[0].Note).Heap() != 0 || c.ParseLeaks(escapeFunc(c, "Async").Type().Params()[0].Note).Heap() != 0 {
		t.Error("heap escape tags lost")
	}
	if c.ParseLeaks(escapeFunc(c, "Deferred").Type().Params()[0].Note).Heap() != -1 {
		t.Error("defer parameter unexpectedly escapes")
	}
	if !escapeLocal(c, "ByValue", "x").Byval() || escapeLocal(c, "ByReference", "x").Byval() {
		t.Error("closure capture classification")
	}
	sawLen, readOnly, writable := false, false, false
	ir.Visit(escapeFunc(c, "SliceLength"), func(n ir.Node) {
		if n, ok := n.(*ir.MakeExpr); ok && n.Op() == ir.OMAKESLICE {
			sawLen = n.Len.Op() == ir.OLITERAL && c.Typecheck.IR.Int64Val(n.Len) == 4 && len(n.Init()) > 0
		}
	})
	ir.Visit(escapeFunc(c, "ReadOnly"), func(n ir.Node) { readOnly = readOnly || n.Op() == ir.OSTR2BYTESTMP })
	ir.Visit(escapeFunc(c, "Writable"), func(n ir.Node) { writable = writable || n.Op() == ir.OSTR2BYTES })
	if !sawLen || !readOnly || !writable {
		t.Errorf("native allocation rewrites: literal=%t readonly=%t writable=%t", sawLen, readOnly, writable)
	}
	if c.Typecheck.IR.CurFunc != nil {
		t.Error("current function leaked")
	}
}
func TestNativeEscapeIsolation(t *testing.T) {
	old := escapeContext(t)
	old.Funcs(old.Typecheck.Target.Funcs)
	before := escapeDump(old)
	for i := 0; i < 8; i++ {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			t.Parallel()
			c := escapeContext(t)
			var trace bytes.Buffer
			c.Typecheck.IR.TraceWriter = &trace
			c.Typecheck.IR.Base.Flag.LowerM = 2
			c.Funcs(c.Typecheck.Target.Funcs)
			if before != escapeDump(old) || !strings.Contains(trace.String(), "escapes to heap") || len(c.Typecheck.IR.Diagnostics.Diagnostics()) == 0 || len(old.Typecheck.IR.Diagnostics.Diagnostics()) != 0 {
				t.Fatal("shared graph or missing instance diagnostics")
			}
		})
	}
}
func TestNativeEscapeRejectsForeignFuncs(t *testing.T) {
	a, b := escapeContext(t), escapeContext(t)
	before := escapeDump(a)
	defer func() {
		if recover() == nil {
			t.Error("foreign function accepted")
		}
		if escapeDump(a) != before {
			t.Error("local graph changed before rejection")
		}
	}()
	a.Funcs(append(append([]*ir.Func(nil), a.Typecheck.Target.Funcs...), b.Typecheck.Target.Funcs[0]))
}
func TestNativeEscapeRuntimeFreegc(t *testing.T) {
	c := escapeContext(t)
	c.Typecheck.IR.Types.Experiment.RuntimeFreegc = true
	c.Typecheck.IR.Debug.EscapeAlias = 4
	c.Typecheck.IR.Debug.FreeAppend = 1
	var trace bytes.Buffer
	c.Typecheck.IR.TraceWriter = &trace
	c.Funcs(c.Typecheck.Target.Funcs)
	if !strings.Contains(trace.String(), "alias analysis") {
		t.Fatal("instance feature did not run native alias analysis")
	}
	for _, name := range []string{"AppendLocal", "AppendAliased"} {
		seen := false
		ir.Visit(escapeFunc(c, name), func(n ir.Node) {
			if call, ok := n.(*ir.CallExpr); ok && call.Op() == ir.OAPPEND {
				seen = true
				if call.AppendNoAlias != (name == "AppendLocal") {
					t.Errorf("%s: native append alias classification", name)
				}
			}
		})
		if !seen {
			t.Fatal("missing append")
		}
	}
}

func TestNativeEscapeExternalDeclarations(t *testing.T) {
	p := sourceContext(t, "example/p", `package p;func External(p *int);func Trusted(p *int);func CallExternal(p *int){External(p)};func CallTrusted(p *int){Trusted(p)}`, false, func(nd *noder.Context) { nd.Flag.Complete = false })
	p.MakeTask()
	tc := p.Noder.Typecheck
	ssagen.NewContext(tc).NewSymABIs().GenABIWrappers()
	deadlocals.NewContext(tc.IR).Funcs(tc.Target.Funcs)
	c := escape.NewContext(tc, p.Noder.Inline.Log)
	escapeFunc(c, "Trusted").Pragma |= ir.Noescape
	c.Funcs(tc.Target.Funcs)
	for _, name := range []string{"External", "CallExternal"} {
		if c.ParseLeaks(escapeFunc(c, name).Type().Params()[0].Note).Heap() != 0 {
			t.Errorf("%s missing external heap leak", name)
		}
	}
	for _, name := range []string{"Trusted", "CallTrusted"} {
		if c.ParseLeaks(escapeFunc(c, name).Type().Params()[0].Note).Heap() != -1 {
			t.Errorf("%s lost noescape annotation", name)
		}
	}
	if escapeFunc(c, "Trusted").Pragma&ir.UintptrKeepAlive == 0 {
		t.Fatal("native external annotation missing")
	}
}
func TestNativeEscapeRunsOnce(t *testing.T) {
	c := escapeContext(t)
	c.Funcs(c.Typecheck.Target.Funcs)
	defer func() {
		if v := recover(); v == nil || !strings.Contains(fmt.Sprint(v), "unexpected node") {
			t.Errorf("reanalysis of classified graph: %v", v)
		}
	}()
	c.Funcs(c.Typecheck.Target.Funcs)
}

func TestNativeEscapeInlinedDiagnosticPosition(t *testing.T) {
	for _, runtime := range []bool{false, true} {
		t.Run(fmt.Sprint(runtime), func(t *testing.T) {
			p := sourceContext(t, "example/p", "package p\nfunc Heap()*int{x:=1;return &x}\nfunc Caller()*int{return Heap()}\n", false, func(nd *noder.Context) { nd.Flag.LowerL = 1 })
			p.MakeTask()
			tc := p.Noder.Typecheck
			ssagen.NewContext(tc).NewSymABIs().GenABIWrappers()
			deadlocals.NewContext(tc.IR).Funcs(tc.Target.Funcs)
			c := escape.NewContext(tc, p.Noder.Inline.Log)
			tc.IR.Base.Flag.LowerM = 1
			tc.IR.Base.Flag.CompilingRuntime = runtime
			c.Funcs(tc.Target.Funcs)
			warning, compilerError := false, false
			for _, d := range tc.IR.Diagnostics.Diagnostics() {
				if d.Line == 3 {
					warning = warning || d.Message == "moved to heap: x"
					compilerError = compilerError || strings.Contains(d.Message, "not allowed in runtime")
				}
			}
			if !warning || compilerError != runtime {
				t.Fatalf("inlined call site diagnostics: %+v", tc.IR.Diagnostics.Diagnostics())
			}
		})
	}
}

package noder

import (
	"fmt"
	"go-backend-ssa/internal/ir"
	"strings"
	"testing"
)

const wrapperSource = `package p
type T struct{N int}
func(t T)V(x int)int{return t.N+x}
func(t *T)P(x int)int{return t.N-x}
func(t T)Pair(x int)(int,int){return t.N,x}
func(t T)Void(){ }
func(t T)Var(xs ...int)int{return len(xs)+t.N}
func(t T)Getter()func(int)int{return t.V}
type Outer struct{T}
type Pointer struct{*T}
type I interface{V(int)int}
type WithInterface struct{I}
type Box[A any]struct{V A}
func(b Box[A])Get()A{return b.V}
func MethodValue(t T)func(int)int{return t.V}
func PointerValue(t *T)func(int)int{return t.P}
func InterfaceValue(t I)func(int)int{return t.V}
func GenericValue(b Box[int])func()int{return b.Get}
func Promoted(o Outer)int{return o.V(1)}
func PromotedPtr(o *Pointer)int{return o.P(2)}
func PromotedInterface(o *WithInterface)int{return o.V(3)}
type N struct{V int}
func(n N)NoHeap()int{return n.V}
`

func wrapperContext(t *testing.T, newInliner bool) *Context {
	t.Helper()
	c := bodyContext(t)
	c.Types.Experiment.NewInliner = newInliner
	c.Flag.LowerL = 1
	c.Debug.InlFuncsWithClosures = 1
	c.ReadLocalPackage(localStub(t, c, wrapperSource))
	driverFunc(t, c, "(*T).P").Pragma |= ir.Noinline
	c.Types.LocalPkg.Lookup("N").Def.Type().SetNotInHeap(true)
	c.Interleaved.DevirtualizeAndInlinePackage(c.Typecheck.Target, nil)
	return c
}
func wrapperFuncs(c *Context) map[string]*ir.Func {
	out := map[string]*ir.Func{}
	for _, fn := range c.Typecheck.Target.Funcs {
		out[fn.Sym().Name] = fn
	}
	return out
}
func TestNativeMakeWrappers(t *testing.T) {
	for _, newInliner := range []bool{false, true} {
		t.Run(fmt.Sprint(newInliner), func(t *testing.T) {
			c := wrapperContext(t, newInliner)
			before := len(c.Typecheck.Target.Funcs)
			c.MakeWrappers(c.Typecheck.Target)
			if len(c.needWrapperTypes)+len(c.haveWrapperTypes)+len(c.needMethodValueWrappers)+len(c.haveMethodValueWrappers) != 0 {
				t.Fatal("wrapper queues not consumed")
			}
			if c.Typecheck.IR.CurFunc != nil {
				t.Fatal("wrapper current function leaked")
			}
			fns := wrapperFuncs(c)
			for _, name := range []string{"(*T).V", "Outer.V", "(*Outer).V", "(*Pointer).P", "T.V-fm", "(*T).P-fm", "I.V-fm", "Box[int].Get-fm", "(*N).NoHeap", "(**N).NoHeap"} {
				if fns[name] == nil {
					t.Fatal("missing wrapper", name, fmt.Sprint(fns))
				}
			}
			for _, fn := range c.Typecheck.Target.Funcs[before:] {
				if fn.Nname.Defn != fn || len(fn.Body) == 0 || !fn.Dupok() {
					t.Fatal("incomplete wrapper", fn)
				}
				for _, stmt := range fn.Body {
					if stmt.Typecheck() == 0 {
						t.Fatal("unchecked wrapper statement", fn, stmt)
					}
				}
				if nodeCounts(fn)[ir.OPAREN] != 0 {
					t.Fatal("optimization parentheses remain")
				}
				for _, d := range fn.Dcl {
					if d.Curfn != fn {
						t.Fatal("foreign wrapper local", fn, d)
					}
				}
			}
			val := fns["T.V-fm"]
			if len(val.ClosureVars) != 1 || !val.Needctxt() || !val.Wrapper() || nodeCounts(val)[ir.OINLCALL] == 0 {
				t.Fatal("method value receiver/inline metadata missing")
			}
			if nodeCounts(fns["(*T).V"])[ir.OIF] == 0 {
				t.Fatal("value receiver panicwrap guard missing")
			}
			if nodeCounts(fns["(*Pointer).P"])[ir.OTAILCALL] != 1 {
				t.Fatal("promoted pointer wrapper did not tailcall")
			}
		})
	}
}
func TestNativeWrapperTailTargets(t *testing.T) {
	for _, tc := range []struct {
		arch    string
		dynlink bool
		tail    bool
	}{{"amd64", false, true}, {"ppc64", false, true}, {"ppc64", true, false}, {"ppc64le", true, false}} {
		t.Run(fmt.Sprintf("%s/%t", tc.arch, tc.dynlink), func(t *testing.T) {
			c := wrapperContext(t, false)
			c.Ctxt.Arch.Name = tc.arch
			c.Ctxt.Flag_dynlink = tc.dynlink
			c.MakeWrappers(c.Typecheck.Target)
			fn := wrapperFuncs(c)["(*Pointer).P"]
			if (nodeCounts(fn)[ir.OTAILCALL] == 1) != tc.tail || fn.Wrapper() == tc.tail {
				t.Fatal("native tailcall target rule", tc, fn, nodeCounts(fn))
			}
		})
	}
}
func TestNativeWrapperIsolation(t *testing.T) {
	old := wrapperContext(t, false)
	old.MakeWrappers(old.Typecheck.Target)
	kept := wrapperFuncs(old)["T.V-fm"]
	before := fmt.Sprint(nodeCounts(kept))
	recv := kept.ClosureVars[0]
	for i := 0; i < 8; i++ {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			t.Parallel()
			c := wrapperContext(t, i%2 != 0)
			c.MakeWrappers(c.Typecheck.Target)
			fn := wrapperFuncs(c)["T.V-fm"]
			if fn == kept || fn.Sym() == kept.Sym() || fn.ClosureVars[0] == recv || fmt.Sprint(nodeCounts(kept)) != before {
				t.Fatal("wrapper graph shared or retained graph changed")
			}
		})
	}
}
func TestNativeWrapperForeignTarget(t *testing.T) {
	a, b := wrapperContext(t, false), wrapperContext(t, false)
	before := len(a.needWrapperTypes)
	defer func() {
		if recover() == nil {
			t.Error("foreign target accepted")
		}
		if len(a.needWrapperTypes) != before {
			t.Error("rejection changed wrapper queues")
		}
	}()
	a.MakeWrappers(b.Typecheck.Target)
}
func TestNativeImportedWrappers(t *testing.T) {
	data := compilerExport(t, "example/dep", `package dep;type D int;func(d D)M(x int)int{return int(d)+x};func Factory(d D)func(int)int{return d.M}`)
	c := bodyContext(t)
	c.Flag.LowerL = 1
	c.ReadLocalPackage(localStub(t, c, `package p;func Caller(x int)func(int)int{return nil}`))
	dep := c.Types.NewPkg("example/dep", "")
	c.RegisterPackage(dep, data, false)
	fn, err := c.LookupFunc("example/dep.Factory")
	if err != nil {
		t.Fatal(err)
	}
	if !c.Typecheck.HaveInlineBody(fn) {
		t.Fatal("imported body missing")
	}
	caller := driverFunc(t, c, "Caller")
	c.Typecheck.IR.WithFunc(caller, func() {
		arg := c.Typecheck.Expr(c.Typecheck.IR.NewConvExpr(caller.Pos(), ir.OCONV, dep.Lookup("D").Def.Type(), caller.Dcl[0]))
		call := c.Typecheck.Call(caller.Pos(), fn.Nname, []ir.Node{arg}, false).(*ir.CallExpr)
		res := c.Inline.TryInlineCall(caller, call, false, nil, false)
		if res == nil {
			t.Fatal("imported factory expansion failed")
		}
		caller.Body[0].(*ir.ReturnStmt).Results[0] = res
	})
	if len(c.haveWrapperTypes) == 0 || len(c.haveMethodValueWrappers) == 0 {
		t.Fatal("import wrapper queue not populated")
	}
	c.MakeWrappers(c.Typecheck.Target)
	for _, fn := range c.Typecheck.Target.Funcs {
		if strings.HasPrefix(ir.LinkFuncName(fn), "example/dep.") {
			t.Fatal("regenerated imported wrapper", fn)
		}
	}
	sym := dep.Lookup("D.M-fm")
	if sym.Def == nil || !sym.Uniq() {
		t.Fatal("imported method value declaration missing")
	}
}

func TestNativeRuntimeErrorWrapper(t *testing.T) {
	for _, runtime := range []bool{false, true} {
		t.Run(fmt.Sprint(runtime), func(t *testing.T) {
			c := bodyContext(t)
			if runtime {
				c.Ctxt.Pkgpath = "runtime"
			}
			c.MakeWrappers(c.Typecheck.Target)
			if (wrapperFuncs(c)["error.Error"] != nil) != runtime {
				t.Fatal("runtime error wrapper ownership")
			}
		})
	}
}

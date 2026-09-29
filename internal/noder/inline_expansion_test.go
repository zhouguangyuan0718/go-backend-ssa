package noder

import (
	"fmt"
	"go-backend-ssa/internal/cmd/pgo"
	"strings"
	"sync"
	"testing"

	"go-backend-ssa/internal/cmd/obj"
	"go-backend-ssa/internal/ir"
)

const inlineSource = `package p
var State int
func Add(x int)int{return x+1}
func Pair(x int)(int,int){return x,x+1}
func Named(x int)(r int){if x>0{r=x};return}
func Early(x int)int{if x>0{return x};return -x}
func Touch(x int){State=x}
func Variadic(xs ...int)int{return len(xs)}
func Identity[T any](x T)T{return x}
func Maker(x int)func()int{return func()int{return x}}
func F(x int)int{return Add(x)}
func Multi(x int)(int,int){return Pair(x)}
func NamedCaller(x int)int{return Named(x)}
func EarlyCaller(x int)int{return Early(x)}
func Void(x int){Touch(x)}
func Varargs(x int)int{return Variadic(x,2,3)}
func Generic(x int)int{return Identity(x)}
func Closure(x int)int{return func(y int)int{return x+y}(2)}
func ReturnedClosure(x int)func()int{return Maker(x)}
func Rec(x int)int{return Rec(x-1)}
func NoInline(x int)int{defer Touch(x);return x}
func Rejected(x int)int{return NoInline(x)}
`

func inlineContext(t *testing.T, newInliner bool) *Context {
	t.Helper()
	c := bodyContext(t)
	c.Types.Experiment.NewInliner = newInliner
	c.Flag.LowerL = 1
	c.Flag.GenDwarfInl = 2
	c.Debug.InlFuncsWithClosures = 1
	c.Ctxt.DwFixups = obj.NewDwarfFixupTable(c.Ctxt)
	c.ReadLocalPackage(localStub(t, c, inlineSource))
	c.Inline.CanInlineFuncs(c.Typecheck.Target.Funcs, nil)
	return c
}

// Each fixture caller has one source call. This exercises the public call-site
// operation; it deliberately does not stand in for the native package driver.
func inlineOne(t *testing.T, c *Context, name string) (*ir.Func, *ir.InlinedCallExpr) {
	t.Helper()
	caller, err := c.LookupFunc("example/p." + name)
	if err != nil {
		t.Fatal(err)
	}
	var call *ir.CallExpr
	ir.VisitList(caller.Body, func(n ir.Node) {
		if v, ok := n.(*ir.CallExpr); ok && call == nil {
			call = v
		}
	})
	if call == nil {
		t.Fatal("missing source call", name)
	}
	callee := c.Inline.InlineCallTarget(caller, call, nil)
	if callee == nil {
		t.Fatal("no eligible callee", name)
	}
	oldDcl := append([]*ir.Name(nil), callee.Dcl...)
	before := len(caller.Dcl)
	var result *ir.InlinedCallExpr
	c.Typecheck.IR.WithFunc(caller, func() {
		c.Inline.Heuristics.ScoreCalls(caller)
		defer c.Inline.Heuristics.ScoreCallsCleanup()
		result = c.Inline.TryInlineCall(caller, call, false, nil, true)
	})
	if result == nil {
		t.Fatal("native expansion refused", name)
	}
	if result.Type() != call.Type() || result.Typecheck() != 1 || len(result.Init()) == 0 || len(result.Body) == 0 || len(caller.Dcl) < before {
		t.Fatal("invalid expansion", name)
	}
	if !callee.Inl.HaveDcl || len(callee.Dcl) != len(oldDcl) {
		t.Fatal("inline declaration expansion damaged callee", name)
	}
	for i, n := range oldDcl {
		if callee.Dcl[i] != n {
			t.Fatal("callee declaration identity changed")
		}
	}
	for _, n := range caller.Dcl[before:] {
		if n.Curfn != caller || n.Class != ir.PAUTO {
			t.Fatal("inline local not reparented", n)
		}
	}
	for _, n := range result.ReturnVars {
		if n.(*ir.Name).Curfn != caller {
			t.Fatal("return not owned by caller")
		}
	}
	for _, n := range callee.Inl.Dcl {
		if n.Curfn != callee {
			t.Fatal("debug declaration not owned by callee")
		}
	}
	if c.Ctxt.DwFixups.GetPrecursorFunc(callee.Linksym()) != callee {
		t.Fatal("native debug precursor missing")
	}
	if c.Typecheck.IR.CurFunc != nil || len(c.todoBodies) != 0 || len(c.todoDicts) != 0 {
		t.Fatal("native reader state leaked")
	}
	return caller, result
}

func TestNativeInlineExpansion(t *testing.T) {
	for _, newInliner := range []bool{false, true} {
		t.Run(fmt.Sprint(newInliner), func(t *testing.T) {
			c := inlineContext(t, newInliner)
			for _, name := range []string{"F", "Multi", "NamedCaller", "EarlyCaller", "Void", "Varargs", "Generic", "Closure", "ReturnedClosure"} {
				t.Run(name, func(t *testing.T) { _, _ = inlineOne(t, c, name) })
			}
			rejected, _ := c.LookupFunc("example/p.Rejected")
			var call *ir.CallExpr
			ir.VisitList(rejected.Body, func(n ir.Node) {
				if v, ok := n.(*ir.CallExpr); ok {
					call = v
				}
			})
			if got := c.Inline.TryInlineCall(rejected, call, false, nil, false); got != nil {
				t.Fatal("recursive target unexpectedly expanded")
			}
		})
	}
}

func TestConcurrentNativeInlineExpansionRetainsGraphs(t *testing.T) {
	const count = 8
	contexts := make([]*Context, count)
	results := make([]*ir.InlinedCallExpr, count)
	texts := make([]string, count)
	var wg sync.WaitGroup
	for i := range count {
		wg.Go(func() {
			c := inlineContext(t, i%2 == 0)
			contexts[i] = c
			_, results[i] = inlineOne(t, c, "F")
			texts[i] = fmt.Sprint(results[i])
		})
	}
	wg.Wait()
	for i, c := range contexts {
		if !c.Typecheck.IR.OwnsNode(results[i]) || fmt.Sprint(results[i]) != texts[i] {
			t.Fatal("old inline graph changed")
		}
		for j, d := range contexts {
			if i != j && (results[i] == results[j] || c.Ctxt.DwFixups == d.Ctxt.DwFixups) {
				t.Fatal("shared inline state")
			}
		}
	}
	foreign, _ := contexts[1].LookupFunc("example/p.Multi")
	var call *ir.CallExpr
	ir.VisitList(foreign.Body, func(n ir.Node) {
		if v, ok := n.(*ir.CallExpr); ok {
			call = v
		}
	})
	defer func() {
		if recover() == nil {
			t.Error("foreign inline request accepted")
		}
	}()
	contexts[0].Inline.TryInlineCall(foreign, call, false, nil, false)
}

func TestImportedNativeInlineExpansion(t *testing.T) {
	data := compilerExport(t, "example/dep", `package dep;func Exported(x int)int{return x+3};func Maker(x int)func()int{return func()int{return x}}`)
	for _, name := range []string{"Exported", "Maker"} {
		t.Run(name, func(t *testing.T) {
			c := inlineContext(t, false)
			c.RegisterPackage(c.Types.NewPkg("example/dep", ""), data, false)
			callee, err := c.LookupFunc("example/dep." + name)
			if err != nil {
				t.Fatal(err)
			}
			if callee.Inl == nil || callee.Inl.HaveDcl || len(callee.Body) != 0 {
				t.Fatal("expected lazy imported inline body")
			}
			caller, err := c.LookupFunc("example/p.F")
			if err != nil {
				t.Fatal(err)
			}
			var result *ir.InlinedCallExpr
			c.Typecheck.IR.WithFunc(caller, func() {
				call := c.Typecheck.Call(caller.Pos(), callee.Nname, []ir.Node{caller.Dcl[0]}, false).(*ir.CallExpr)
				result = c.Inline.TryInlineCall(caller, call, false, nil, false)
			})
			if result == nil || !callee.Inl.HaveDcl || len(callee.Inl.Dcl) == 0 || len(callee.Body) != 0 {
				t.Fatal("imported expansion or lazy body ownership")
			}
			for _, fn := range c.Typecheck.Target.Funcs {
				if fn == callee {
					t.Fatal("ordinary import was added to local compiler queue")
				}
			}
			if c.Ctxt.DwFixups.GetPrecursorFunc(callee.Linksym()) != callee {
				t.Fatal("imported precursor missing")
			}
			for _, n := range callee.Inl.Dcl {
				if n.Curfn != callee {
					t.Fatal("imported abstract debug declaration owner")
				}
			}
		})
	}
}

func TestNativeNestedCalleeEffects(t *testing.T) {
	c := bodyContext(t)
	c.Flag.LowerL = 1
	c.Debug.InlFuncsWithClosures = 1
	c.ReadLocalPackage(localStub(t, c, `package p;var State int;func Maker(x int)func()int{State++;return func()int{return x}};func F(x int)int{return Maker(x)()}`))
	c.Inline.CanInlineFuncs(c.Typecheck.Target.Funcs, nil)
	caller, err := c.LookupFunc("example/p.F")
	if err != nil {
		t.Fatal(err)
	}
	expansions := 0
	var edit func(ir.Node) ir.Node
	edit = func(n ir.Node) ir.Node {
		ir.EditChildren(n, edit)
		if call, ok := n.(*ir.CallExpr); ok {
			if inlined := c.Inline.TryInlineCall(caller, call, false, nil, true); inlined != nil {
				expansions++
				return inlined
			}
		}
		return n
	}
	c.Typecheck.IR.WithFunc(caller, func() { ir.EditChildren(caller, edit) })
	if expansions != 2 {
		t.Fatal("nested source calls did not expand", expansions)
	}
	effects, calls := 0, 0
	ir.VisitList(caller.Body, func(n ir.Node) {
		if as, ok := n.(*ir.AssignOpStmt); ok && as.X.Sym() != nil && as.X.Sym().Name == "State" {
			effects++
		}
		if _, ok := n.(*ir.CallExpr); ok {
			calls++
		}
	})
	if effects != 1 || calls != 0 {
		t.Fatal("callee side effect duplicated/lost or calls retained", effects, calls)
	}
}

func TestNativeInlineRecursionStopsAtRepeatedCall(t *testing.T) {
	c := inlineContext(t, false)
	fn, err := c.LookupFunc("example/p.Rec")
	if err != nil {
		t.Fatal(err)
	}
	var call *ir.CallExpr
	ir.VisitList(fn.Body, func(n ir.Node) {
		if v, ok := n.(*ir.CallExpr); ok {
			call = v
		}
	})
	c.Typecheck.IR.WithFunc(fn, func() {
		first := c.Inline.TryInlineCall(fn, call, false, nil, false)
		if first == nil {
			t.Fatal("first native recursion expansion refused")
		}
		var nested *ir.CallExpr
		ir.Visit(first, func(n ir.Node) {
			if v, ok := n.(*ir.CallExpr); ok {
				nested = v
			}
		})
		if nested == nil {
			t.Fatal("recursive call lost")
		}
		before := c.inlgen
		if again := c.Inline.TryInlineCall(fn, nested, false, nil, false); again != nil || c.inlgen != before {
			t.Fatal("native recursion guard failed")
		}
	})
}

func TestNativePGOInlineExpansion(t *testing.T) {
	for _, budget := range []int{100, 400} {
		t.Run(fmt.Sprint(budget), func(t *testing.T) {
			c := bodyContext(t)
			c.Flag.LowerL = 1
			c.Debug.PGOInlineBudget = budget
			c.ReadLocalPackage(localStub(t, c, "package p;func Big(x int)int{"+strings.Repeat("x++;", 50)+"return x};func Caller(x int)int{return Big(x)}"))
			input, err := pgo.FromSerialized(strings.NewReader("GO PREPROFILE V1\nexample/p.Caller\nexample/p.Big\n0 100\n"))
			if err != nil {
				t.Fatal(err)
			}
			profile := c.Inline.PGO.NewProfile(input)
			c.Inline.CanInlineFuncs(c.Typecheck.Target.Funcs, profile)
			caller, err := c.LookupFunc("example/p.Caller")
			if err != nil {
				t.Fatal(err)
			}
			var call *ir.CallExpr
			ir.VisitList(caller.Body, func(n ir.Node) {
				if v, ok := n.(*ir.CallExpr); ok {
					call = v
				}
			})
			c.Typecheck.IR.WithFunc(caller, func() {
				result := c.Inline.TryInlineCall(caller, call, false, profile, false)
				if (result != nil) != (budget == 400) || c.Inline.HasPgoHotInline(caller) != (budget == 400) {
					t.Fatal("native hot-call expansion state", budget, result)
				}
			})
		})
	}
}

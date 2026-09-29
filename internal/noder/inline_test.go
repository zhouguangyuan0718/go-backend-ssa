package noder

import (
	"bytes"
	"fmt"
	"go-backend-ssa/internal/cmd/pgo"
	"go-backend-ssa/internal/ir"
	"go-backend-ssa/internal/logopt"
	"strings"
	"sync"
	"testing"
)

func TestNativeInlinabilityFromSource(t *testing.T) {
	const count = 8
	var wg sync.WaitGroup
	retained := make([]*ir.Inline, count)
	for i := range count {
		wg.Go(func() {
			c := bodyContext(t)
			c.Flag.LowerL = 1
			c.Types.Experiment.NewInliner = i%2 == 0
			c.Inline.Log.Format = logopt.Json0
			var trace bytes.Buffer
			c.Typecheck.IR.TraceWriter = &trace
			objects := readTestSource(t, c, `package p
   func Small(x int)int{return x+1}
   func Caller(x int)int{return Small(x)}
   func No(x int)int{return x}
   func Defer(ch chan int){defer close(ch)}
  `, "Small", "Caller", "No", "Defer")
			objects["No"].Func.Pragma |= ir.Noinline
			c.Debug.AstDump = "Small"
			c.Inline.CanInlineFuncs(c.Typecheck.Target.Funcs, nil)
			small, caller := objects["Small"].Func, objects["Caller"].Func
			if small.Inl == nil || caller.Inl == nil || objects["No"].Func.Inl != nil || objects["Defer"].Func.Inl != nil {
				t.Error("native inlinability lost")
				return
			}
			if !strings.Contains(trace.String(), "inline, OK") {
				t.Error("missing native AST dump")
			}
			if small.Inl.Cost != 4 || !small.Inl.HaveDcl {
				t.Errorf("small inline metadata: %+v", small.Inl)
			}
			if c.Types.Experiment.NewInliner && small.Inl.Properties == "" {
				t.Error("heuristics not called")
			}
			retained[i] = small.Inl
			var events []string
			for _, event := range c.Inline.Log.Snapshot() {
				events = append(events, fmt.Sprint(event.FunctionName(), ":", event.What(), ":", event.Arguments()))
			}
			text := strings.Join(events, "\n")
			for _, want := range []string{"Small:canInlineFunction:[cost: 4]", "No:cannotInlineFunction:[marked go:noinline]", "Defer:cannotInlineFunction:"} {
				if !strings.Contains(text, want) {
					t.Error("missing native event", want, text)
				}
			}
			c.Inline.CanInlineFuncs(c.Typecheck.Target.Funcs, nil)
			if small.Inl != retained[i] {
				t.Error("reanalysis replaced checked inline metadata")
			}
			foreign := bodyContext(t)
			func() {
				defer func() {
					if recover() == nil {
						t.Error("foreign function accepted")
					}
				}()
				foreign.Inline.CanInline(small, nil)
			}()
			unnamed := c.Typecheck.IR.NewFunc(small.Pos(), small.Pos(), c.Types.LocalPkg.Lookup("Unnamed"), small.Type())
			unnamed.Nname = nil
			if c.Inline.InlineImpossible(unnamed) != "no name" {
				t.Error("unnamed native rejection changed")
			}
		})
	}
	wg.Wait()
	for i, p := range retained {
		if p == nil {
			continue
		}
		if p.Cost != 4 {
			t.Error("retained graph changed")
		}
		for j := 0; j < i; j++ {
			if p == retained[j] {
				t.Error("inline metadata shared")
			}
		}
	}
}

func TestNativeReadBodiesDuringInlining(t *testing.T) {
	for _, shared := range []bool{false, true} {
		t.Run(fmt.Sprint(shared), func(t *testing.T) {
			c := bodyContext(t)
			c.Flag.LowerL = 1
			c.Flag.LowerM = 2
			c.Ctxt.Flag_linkshared = shared
			readTestSourceMode(t, c, `package p
   func Id[T any](x T)T{return x}
   type Box[T any]struct{V T}
   func(b Box[T])Get()T{return b.V}
   func Generic()int{return Id(Box[int]{V:3}).Get()}
  `, true, "Generic")
			if c.Flag.LowerM != 2 || c.todoBodies != nil || c.todoDicts != nil {
				t.Fatal("driver state not restored/drained")
			}
			released, kept := 0, 0
			for fn, pri := range c.bodyReader {
				if fn.OClosure != nil || len(pri.dict.targs) == 0 {
					continue
				}
				if shared && ir.IsMethod(fn) {
					if len(fn.Body) == 0 {
						t.Errorf("linkshared method released: %v", fn.Sym())
					}
					kept++
					continue
				}
				if len(fn.Body) != 0 || !fn.InlinabilityChecked() || fn.Inl == nil {
					t.Errorf("generic not analyzed/released: %v body=%d checked=%t", fn.Sym(), len(fn.Body), fn.InlinabilityChecked())
				}
				released++
			}
			if released == 0 || shared && kept == 0 {
				t.Fatal("generic driver branches untested", released, kept)
			}
		})
	}
}

func TestNativePGOInlinability(t *testing.T) {
	for _, budget := range []int{100, 400} {
		t.Run(fmt.Sprint(budget), func(t *testing.T) {
			c := bodyContext(t)
			c.Flag.LowerL = 1
			c.Debug.PGOInlineBudget = budget
			objects := readTestSource(t, c, "package p;func Big(x int)int{"+strings.Repeat("x++;", 50)+"return x};func Caller(x int)int{return Big(x)}", "Big", "Caller")
			input, err := pgo.FromSerialized(strings.NewReader("GO PREPROFILE V1\nexample/p.Caller\nexample/p.Big\n0 100\n"))
			if err != nil {
				t.Fatal(err)
			}
			profile := c.Inline.PGO.NewProfile(input)
			c.Inline.CanInlineFuncs(c.Typecheck.Target.Funcs, profile)
			big := objects["Big"].Func
			if !c.Inline.IsPgoHotFunc(big, profile) || (big.Inl != nil) != (budget == 400) {
				t.Fatal("native PGO budget", budget, big.Inl)
			}
			other := bodyContext(t)
			defer func() {
				if recover() == nil {
					t.Error("foreign PGO graph accepted")
				}
			}()
			other.Inline.PGOInlinePrologue(profile)
		})
	}
}

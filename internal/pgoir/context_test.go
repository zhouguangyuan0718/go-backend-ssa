package pgoir

import (
	"bytes"
	"fmt"
	"strings"
	"sync"
	"testing"

	"go-backend-ssa/internal/cmd/obj"
	"go-backend-ssa/internal/cmd/pgo"
	"go-backend-ssa/internal/cmd/src"
	"go-backend-ssa/internal/ir"
	"go-backend-ssa/internal/typecheck"
	"go-backend-ssa/internal/types"
)

func graphContext() (*Context, *ir.Func) {
	link := &obj.Link{}
	u := types.NewUniverseWithPositions(8, 8, 1<<50, &link.PosTable)
	u.LocalPkg = u.NewPkg("example/p", "p")
	graph := ir.NewContext(u, link)
	tc := typecheck.NewContext(graph)
	tc.InitUniverse()
	pos := link.PosTable.XPos(src.MakePos(src.NewFileBase("p.go", "/src/p.go"), 10, 1))
	fn := graph.NewFunc(pos, pos, u.LocalPkg.Lookup("F"), u.NewSignature(nil, nil, nil))
	fn.Nname.Defn = fn
	tc.Target.Funcs = []*ir.Func{fn}
	return NewContext(tc), fn
}

func TestIndependentAttribution(t *testing.T) {
	input, err := pgo.FromSerialized(strings.NewReader("GO PREPROFILE V1\nexample/p.F\nremote.F\n3 90\nexample/p.F\nmissing.F\n4 10\n"))
	if err != nil {
		t.Fatal(err)
	}
	const count = 16
	profiles := make([]*Profile, count)
	var wg sync.WaitGroup
	for i := range count {
		wg.Go(func() {
			c, fn := graphContext()
			var trace bytes.Buffer
			c.Typecheck.IR.TraceWriter = &trace
			c.Typecheck.IR.Debug.PGODebug = 3
			var looked []string
			cleanup := 0
			// Test bindings exercise the original hooks. Production noder binding is
			// still pending; this is not a substitute for its export-data reader.
			c.LookupFunc = func(name string) (*ir.Func, error) {
				looked = append(looked, name)
				if name == "remote.F" {
					return fn, nil
				}
				return nil, fmt.Errorf("instance %d", i)
			}
			c.PostLookupCleanup = func() { cleanup++ }
			p := c.NewProfile(input)
			profiles[i] = p
			if fmt.Sprint(looked) != "[remote.F missing.F]" || cleanup != 1 {
				t.Errorf("lookup ordering or cleanup: %v %d", looked, cleanup)
			}
			caller := p.WeightedCG.IRNodes["example/p.F"]
			if caller.AST != fn || p.WeightedCG.IRNodes["missing.F"].AST != nil || p.WeightedCG.IRNodes["remote.F"].AST != fn {
				t.Error("native node attribution lost")
			}
			p.PrintWeightedCallGraphDOT(50)
			for _, want := range []string{fmt.Sprintf("instance %d", i), "digraph G", `style=dashed`, `label="90.00"`, "color=red"} {
				if !strings.Contains(trace.String(), want) {
					t.Errorf("trace missing %q: %s", want, trace.String())
				}
			}
			// A repeated graph build must not modify the retained graph or maps.
			next := c.NewProfile(input)
			if next == p || next.WeightedCG == p.WeightedCG || next.WeightedCG.IRNodes["example/p.F"] == caller {
				t.Error("graph reused")
			}
		})
	}
	wg.Wait()
	key := input.NamedEdgeMap.ByWeight[0]
	input.NamedEdgeMap.Weight[key] = 999
	input.NamedEdgeMap.ByWeight[0] = pgo.NamedCallEdge{}
	for i, p := range profiles {
		if p.NamedEdgeMap.Weight[key] != 90 || p.NamedEdgeMap.ByWeight[0] != key {
			t.Fatal("retained input aliased")
		}
		for j := 0; j < i; j++ {
			if profiles[j].WeightedCG.IRNodes["example/p.F"].AST == p.WeightedCG.IRNodes["example/p.F"].AST {
				t.Fatal("IR shared across compilations")
			}
		}
	}
}

func TestMissingBindingsAndForeignGraphs(t *testing.T) {
	c, fn := graphContext()
	other, foreign := graphContext()
	assertPanic := func(name string, f func()) {
		t.Helper()
		t.Run(name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Error("expected rejection")
				}
			}()
			f()
		})
	}
	input, err := pgo.FromSerialized(strings.NewReader("GO PREPROFILE V1\nexample/p.F\nremote.F\n3 90\n"))
	if err != nil {
		t.Fatal(err)
	}
	if c.NewProfile(nil) != nil || c.NewProfile(&pgo.Profile{}) != nil {
		t.Fatal("empty input")
	}
	assertPanic("unbound lookup", func() { c.NewProfile(input) })
	c.LookupFunc = func(string) (*ir.Func, error) { return fn, nil }
	assertPanic("unbound cleanup", func() { c.NewProfile(input) })
	c.PostLookupCleanup = func() {}
	c.LookupFunc = func(string) (*ir.Func, error) { return foreign, nil }
	assertPanic("foreign lookup", func() { c.NewProfile(input) })
	assertPanic("foreign callee", func() { c.DirectCallee(foreign.Nname) })
	assertPanic("foreign positions", func() { c.NodeLineOffset(foreign.Nname, fn) })
	assertPanic("foreign SCC roots", func() { other.Typecheck.IR.VisitFuncsBottomUp([]*ir.Func{fn}, func([]*ir.Func, bool) {}) })
	// Even a foreign function reached through a local root must be rejected.
	fn.Body = ir.Nodes{foreign.Nname}
	assertPanic("foreign SCC dependency", func() { c.Typecheck.IR.VisitFuncsBottomUp([]*ir.Func{fn}, func([]*ir.Func, bool) {}) })
}

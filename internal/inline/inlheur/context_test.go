package inlheur

import (
	"bytes"
	"fmt"
	"go-backend-ssa/internal/cmd/obj"
	"go-backend-ssa/internal/cmd/src"
	"go-backend-ssa/internal/ir"
	"go-backend-ssa/internal/pgoir"
	"go-backend-ssa/internal/typecheck"
	"go-backend-ssa/internal/types"
	"strings"
	"sync"
	"testing"
)

func testGraph() (*Context, *ir.Func, *ir.Func, *ir.CallExpr) {
	link := &obj.Link{}
	u := types.NewUniverseWithPositions(8, 8, 1<<50, &link.PosTable)
	u.LocalPkg = u.NewPkg("example/p", "p")
	graph := ir.NewContext(u, link)
	tc := typecheck.NewContext(graph)
	tc.InitUniverse()
	c := NewContext(tc, pgoir.NewContext(tc))
	pos := link.PosTable.XPos(src.MakePos(src.NewFileBase("input.go", "/src/input.go"), 10, 1))
	sig := u.NewSignature(nil, []*types.Field{u.NewField(pos, u.LocalPkg.Lookup("x"), u.Types[types.TINT])}, []*types.Field{u.NewField(pos, nil, u.Types[types.TINT])})
	fn := graph.NewFunc(pos, pos, u.LocalPkg.Lookup("F"), sig)
	fn.DeclareParams(true)
	fn.Nname.Defn = fn
	cond := graph.NewBinaryExpr(pos, ir.OGT, fn.Dcl[0], graph.NewInt(pos, 0))
	cond.SetType(u.Types[types.TBOOL])
	cond.SetTypecheck(1)
	ret := func() ir.Node { return graph.NewReturnStmt(pos, []ir.Node{graph.NewInt(pos, 7)}) }
	fn.Body = ir.Nodes{graph.NewIfStmt(pos, cond, []ir.Node{ret()}, nil), ret()}
	fn.Inl = &ir.Inline{Cost: 50}
	fn.SetInlinabilityChecked(true)
	caller := graph.NewFunc(pos, pos, u.LocalPkg.Lookup("Caller"), u.NewSignature(nil, nil, nil))
	caller.Nname.Defn = caller
	call := graph.NewCallExpr(pos, ir.OCALLFUNC, fn.Nname, []ir.Node{graph.NewInt(pos, 3)})
	call.SetType(u.Types[types.TINT])
	call.SetTypecheck(1)
	caller.Body = ir.Nodes{call}
	caller.Inl = &ir.Inline{Cost: 50}
	caller.SetInlinabilityChecked(true)
	tc.Target.Funcs = []*ir.Func{fn, caller}
	return c, fn, caller, call
}

func TestIndependentAnalysisAndScoring(t *testing.T) {
	const count = 16
	retained := make([]string, count)
	contexts := make([]*Context, count)
	var wg sync.WaitGroup
	for i := range count {
		wg.Go(func() {
			c, fn, caller, call := testGraph()
			contexts[i] = c
			c.Typecheck.IR.Debug.InlScoreAdj = fmt.Sprintf("passConstToIfAdj:%d", -10-i)
			c.SetupScoreAdjustments()
			canInline := func(*ir.Func) { t.Error("unexpected closure in phase fixture") }
			budget := func(*ir.Func) int32 { return 2000 }
			c.AnalyzeFunc(fn, canInline, budget, 80)
			c.AnalyzeFunc(caller, canInline, budget, 80)
			props := DeserializeFromString(fn.Inl.Properties)
			if len(props.ParamFlags) != 1 || props.ParamFlags[0] != ParamFeedsIfOrSwitch || len(props.ResultFlags) != 1 || props.ResultFlags[0] != ResultAlwaysSameConstant {
				t.Errorf("properties: %s", props)
			}
			c.ScoreCalls(caller)
			score, ok := c.GetCallSiteScore(caller, call)
			if !ok || score != 40-i {
				t.Errorf("score=%d found=%t want=%d", score, ok, 40-i)
			}
			retained[i] = fn.Inl.Properties
			// Re-score a non-candidate to exercise reusable temporary tables and cleanup.
			c.ScoreCallsCleanup()
			delete(c.fpmap, caller)
			caller.Inl = nil
			c.ScoreCalls(caller)
			score, ok = c.GetCallSiteScore(caller, call)
			if !ok || score != 40-i {
				t.Error("uncached score", score, ok)
			}
			c.ScoreCallsCleanup()
			c.TearDown()
			c.AnalyzeFunc(fn, canInline, budget, 80)
			if fn.Inl.Properties != retained[i] || c.fpmap != nil || c.scoreCallsCache.tab != nil {
				t.Error("teardown changed retained properties or kept scratch state")
			}
		})
	}
	wg.Wait()
	for i, c := range contexts {
		if c.adjValues[passConstToIfAdj] != -10-i {
			t.Fatal("shared adjustments")
		}
		if DeserializeFromString(retained[i]).ParamFlags[0] != ParamFeedsIfOrSwitch {
			t.Fatal("retained serialization")
		}
	}
}

func TestAnalysisFailuresAndTrace(t *testing.T) {
	c, fn, caller, call := testGraph()
	foreign, ff, _, _ := testGraph()
	mustPanic := func(name string, f func()) {
		t.Run(name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Error("expected failure")
				}
			}()
			f()
		})
	}
	mustPanic("dependency", func() { NewContext(c.Typecheck, foreign.PGO) })
	mustPanic("foreign function", func() { c.ScoreCalls(ff) })
	c.Typecheck.IR.Debug.InlScoreAdj = "notAnAdjustment:7"
	mustPanic("malformed adjustments", func() { c.SetupScoreAdjustments() })
	c.Typecheck.IR.Debug.InlScoreAdj = ""
	c.Typecheck.IR.Debug.DumpInlCallSiteScores = 1
	c.AnalyzeFunc(fn, func(*ir.Func) {}, func(*ir.Func) int32 { return 2000 }, 80)
	c.AnalyzeFunc(caller, func(*ir.Func) {}, func(*ir.Func) int32 { return 2000 }, 80)
	c.ScoreCalls(caller)
	c.ScoreCallsCleanup()
	var out bytes.Buffer
	c.Typecheck.IR.TraceWriter = &out
	c.DumpInlCallSiteScores(nil, func(*ir.Func, *pgoir.Profile) (int32, bool) { return 80, false })
	if !strings.Contains(out.String(), "# scores for package example/p") || !strings.Contains(out.String(), "F") {
		t.Fatal(out.String())
	}
	nested := c.Typecheck.IR.NewCallExpr(call.Pos(), ir.OCALLFUNC, fn.Nname, []ir.Node{c.Typecheck.IR.NewInt(call.Pos(), 4)})
	nested.SetType(call.Type())
	nested.SetTypecheck(1)
	ic := c.Typecheck.IR.NewInlinedCallExpr(call.Pos(), []ir.Node{nested}, nil)
	c.UpdateCallsiteTable(caller, call, ic)
	tab := c.fpmap[caller].cstab
	if tab[nested] == nil || tab[nested].parent != tab[call] || tab[call].aux&csAuxInlined == 0 {
		t.Fatal("inlined-region call site update")
	}
	if score, ok := c.GetCallSiteScore(caller, nested); !ok || score != 30 {
		t.Fatal("nested score", score, ok)
	}
	if _, ok := c.GetCallSiteScore(caller, call); !ok {
		t.Fatal("lost cached score")
	}
}

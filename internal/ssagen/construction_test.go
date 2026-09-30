package ssagen

import (
	"go-backend-ssa/internal/ir"
	"go-backend-ssa/internal/ssa"
	"testing"
)

func TestConstructionLifecycle(t *testing.T) {
	makeFunction := func(c *Context, name string) *ir.Func {
		fn := frontendFunction(c, name)
		c.Typecheck.IR.InitLSym(fn, true)
		fn.Body = []ir.Node{c.Typecheck.IR.NewReturnStmt(fn.Pos(), nil)}
		return fn
	}
	mustPanic := func(t *testing.T, run func()) {
		t.Helper()
		defer func() {
			if recover() == nil {
				t.Error("expected rejection")
			}
		}()
		run()
	}
	t.Run("foreign-before-mutation", func(t *testing.T) {
		c, other := buildContext("amd64"), buildContext("amd64")
		fn := makeFunction(other, "foreign")
		cache := &c.ssaCaches[0]
		mustPanic(t, func() { c.ConstructSSA(fn, false) })
		if &c.ssaCaches[0] != cache || c.constructionCapture != nil || fn.DebugInfo != nil {
			t.Fatal("foreign input changed construction state")
		}
		held := c.ConstructSSA(makeFunction(c, "valid"), false)
		if &c.ssaCaches[0] != cache || c.constructionCapture != nil || held.Func().Cache == cache {
			t.Fatal("worker cache was not restored")
		}
		held.ContinueNative()
	})
	t.Run("failed-tail-consumed", func(t *testing.T) {
		c := buildContext("amd64")
		held := c.ConstructSSA(makeFunction(c, "failed"), false)
		calls := 0
		held.state.continuation = func() *ssa.Func { calls++; panic("backend failure") }
		mustPanic(t, func() { held.ContinueNative() })
		mustPanic(t, func() { held.ContinueNative() })
		if calls != 1 || held.NativeCompleted() {
			t.Fatal("failed continuation can be reused")
		}
		c.ConstructSSA(makeFunction(c, "next"), false).ContinueNative()
	})
	t.Run("copied-handles-share-consumption", func(t *testing.T) {
		c := buildContext("amd64")
		held := c.ConstructSSA(makeFunction(c, "copied"), false)
		copied := *held
		f := copied.ContinueNative()
		if f != held.Func() || !held.NativeCompleted() || !copied.NativeCompleted() {
			t.Fatal("copied handle lost graph or completion")
		}
		mustPanic(t, func() { held.ContinueNative() })
		mustPanic(t, func() { copied.ContinueNative() })
	})
	t.Run("configuration-changed", func(t *testing.T) {
		c := buildContext("amd64")
		held := c.ConstructSSA(makeFunction(c, "changed"), false)
		config := c.ssaConfig
		c.ssaConfig = buildContext("amd64").ssaConfig
		mustPanic(t, func() { held.ContinueNative() })
		if held.state.started || held.NativeCompleted() || held.Func().RegAlloc != nil {
			t.Fatal("rejected continuation mutated graph")
		}
		c.ssaConfig = config
		held.ContinueNative()
	})
}

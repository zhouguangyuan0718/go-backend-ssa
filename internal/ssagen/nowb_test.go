package ssagen

import (
	"strings"
	"testing"

	"go-backend-ssa/internal/ir"
)

func TestNativeNoWriteBarrierCallGraphIsolation(t *testing.T) {
	makeGraph := func() *Context {
		c := buildContext("amd64")
		root, child := frontendFunction(c, "root"), frontendFunction(c, "child")
		c.Typecheck.IR.InitLSym(root, true)
		c.Typecheck.IR.InitLSym(child, true)
		root.Pragma |= ir.Nowritebarrierrec
		child.WBPos = child.Pos()
		c.Typecheck.Target = &ir.Package{Funcs: []*ir.Func{root, child}}
		c.EnableNoWriteBarrierRecCheck()
		c.nowritebarrierrecCheck.recordCall(root, child.LSym, root.Pos())
		return c
	}
	a, b := makeGraph(), makeGraph()
	b.Typecheck.Target.Funcs[1].Pragma |= ir.Yeswritebarrierrec
	a.NoWriteBarrierRecCheck()
	b.NoWriteBarrierRecCheck()
	da, db := a.Typecheck.IR.Diagnostics.Diagnostics(), b.Typecheck.IR.Diagnostics.Diagnostics()
	if len(da) != 1 || len(db) != 0 || !strings.Contains(da[0].String(), "write barrier prohibited by caller") || !strings.Contains(da[0].String(), "called by root") {
		t.Fatalf("native barrier diagnostics: %v / %v", da, db)
	}
	if a.nowritebarrierrecCheck != nil || b.nowritebarrierrecCheck != nil {
		t.Fatal("checker did not release its graph")
	}
}

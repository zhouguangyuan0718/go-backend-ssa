package typecheck

import (
	"fmt"
	"go-backend-ssa/diagnostic"
	"go-backend-ssa/internal/cmd/src"
	"go-backend-ssa/internal/ir"
	"go-backend-ssa/internal/types"
	"sync"
	"testing"
)

func TestConcurrentNativeCheckingAndRetainedBodies(t *testing.T) {
	const count = 12
	contexts := make([]*Context, count)
	var wg sync.WaitGroup
	for i := range count {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c := newTestContext(4 + 4*(i%2))
			c.InitUniverse()
			contexts[i] = c
			g, u := c.IR, c.IR.Types
			pos := u.PosTable.XPos(src.MakePos(src.NewFileBase(fmt.Sprintf("f%d.go", i), ""), 7, 3))
			sig := u.NewSignature(nil, nil, []*types.Field{u.NewField(pos, nil, u.Types[types.TINT])})
			fn := g.NewFunc(pos, pos, c.Lookup("F"), sig)
			c.DeclFunc(fn)
			tmp := c.TempAt(pos, fn, u.Types[types.TINT])
			sum := c.Expr(g.NewBinaryExpr(pos, ir.OADD, tmp, g.NewInt(pos, 3)))
			fn.Body = []ir.Node{c.Stmt(g.NewAssignStmt(pos, tmp, sum)), c.Stmt(g.NewReturnStmt(pos, []ir.Node{tmp}))}
			inner := g.NewFunc(pos, pos, c.Lookup("Inner"), u.NewSignature(nil, nil, nil))
			c.DeclFunc(inner)
			innerTmp := c.TempAt(pos, inner, u.Types[types.TSTRING])
			c.FinishFuncBody()
			if g.CurFunc != fn || innerTmp.Curfn != inner {
				t.Error("nested declaration stack leaked")
			}
			c.FinishFuncBody()
			c.CheckFuncStack()
			if g.CurFunc != nil || u.Pos != (src.XPos{}) {
				t.Error("shared current state not restored")
			}
			c.SetBaseTypeIndex(u.Types[types.TINT], int64(i+1), int64(i+100))
			checkEmbeddedSelection(t, c, pos)
			tmp.SetByval(true)
			iter := c.NewClosureStructIter([]*ir.Name{tmp, innerTmp})
			n, typ, offset := iter.Next()
			size := int64(4 + 4*(i%2))
			if n != tmp || typ != tmp.Type() || offset != size {
				t.Error("capture value layout")
			}
			n, typ, offset = iter.Next()
			if n != innerTmp || !typ.IsPtr() || typ.Elem() != innerTmp.Type() || offset != 2*size {
				t.Error("capture reference layout")
			}
			n, _, _ = iter.Next()
			if n != nil {
				t.Error("capture iterator termination")
			}
		}()
	}
	wg.Wait()
	for i, c := range contexts {
		u := c.IR.Types
		fn := c.Target.Funcs[0]
		if len(c.Target.Funcs) != 2 || len(fn.Body) != 2 || c.IR.Diagnostics.HasErrors() {
			t.Fatal("retained body/diagnostics")
		}
		tmp := fn.Body[0].(*ir.AssignStmt).X.(*ir.Name)
		sum := fn.Body[0].(*ir.AssignStmt).Y.(*ir.BinaryExpr)
		if sum.Type() != u.Types[types.TINT] || sum.Y.Type() != tmp.Type() || sum.Typecheck() != 1 || tmp.Type().Size() != int64(4+4*(i%2)) {
			t.Fatal("retained native type identity/layout")
		}
		if !tmp.AutoTemp() || !tmp.Used() || tmp.Esc() != ir.EscNever || tmp.Curfn != fn {
			t.Fatal("native temporary flags")
		}
		if c.BaseTypeIndex(u.Types[types.TINT]) != int64(i+1) || c.BaseTypeIndex(u.NewPtr(u.Types[types.TINT])) != int64(i+100) {
			t.Fatal("import index leaked")
		}
		if i > 0 && (tmp.Sym() == contexts[0].Target.Funcs[0].Body[0].(*ir.AssignStmt).X.Sym() || c.IR.Types.LocalPkg == contexts[0].IR.Types.LocalPkg) {
			t.Fatal("temporary/package shared")
		}
	}
}

func TestNativeCheckingDiagnostics(t *testing.T) {
	c := newTestContext(8)
	c.InitUniverse()
	other := newTestContext(8)
	other.InitUniverse()
	g, u := c.IR, c.IR.Types
	pos := u.PosTable.XPos(src.MakePos(src.NewFileBase("invalid.go", "/src/invalid.go"), 11, 4))
	bad := c.Expr(g.NewBinaryExpr(pos, ir.OADD, g.NewInt(pos, 1), g.NewBool(pos, true)))
	ds := g.Diagnostics.Diagnostics()
	if bad.Type() != nil || len(ds) != 1 || ds[0].Filename != "invalid.go" || ds[0].Line != 11 || ds[0].Column != 4 || c.errors() != 1 {
		t.Fatalf("native error: %v / %v", bad, ds)
	}
	if other.errors() != 0 || u.Pos != (src.XPos{}) {
		t.Fatal("diagnostics/position escaped instance")
	}
	// The native selector path can refine a previously emitted undefined error.
	c.errorfAt(pos, 0, "undefined: missing")
	before := g.Diagnostics.Diagnostics()
	c.updateErrorDot(c.IR.Base.FmtPos(pos), "missing", "a.missing")
	after := g.Diagnostics.Diagnostics()
	if before[1].Message != "undefined: missing" || after[1].Message != "undefined: missing in a.missing" {
		t.Fatal("error refinement/snapshot")
	}
	c.errorfAt(pos, 0, "undefined: field")
	g.Diagnostics.Add(diagnostic.Error, "", u.PosTable.Pos(pos), "intervening error")
	c.updateErrorDot(c.IR.Base.FmtPos(pos), "field", "a.field")
	after = g.Diagnostics.Diagnostics()
	if after[len(after)-1].Message != "intervening error" {
		t.Fatal("overwrote intervening diagnostic")
	}
	u.Diagnostics.Add(diagnostic.Error, "", u.PosTable.Pos(pos), "layout error")
	if c.errors() != len(after)+1 {
		t.Fatal("layout errors invisible to typecheck")
	}
}

func checkEmbeddedSelection(t *testing.T, c *Context, pos src.XPos) {
	t.Helper()
	g, u := c.IR, c.IR.Types
	obj := g.NewDeclNameAt(pos, ir.OTYPE, c.Lookup("Embedded"))
	embedded := u.NewNamed(obj)
	obj.SetType(embedded)
	obj.SetTypecheck(1)
	member := u.NewField(pos, c.Lookup("Member"), u.Types[types.TINT])
	embedded.SetUnderlying(u.NewStruct([]*types.Field{member}))
	method := u.NewField(pos, c.Lookup("Method"), u.NewSignature(u.NewField(pos, c.Lookup("recv"), embedded), nil, nil))
	embedded.SetMethods([]*types.Field{method})
	outerObj := g.NewDeclNameAt(pos, ir.OTYPE, c.Lookup("Outer"))
	outer := u.NewNamed(outerObj)
	outerObj.SetType(outer)
	outerObj.SetTypecheck(1)
	field := u.NewField(pos, obj.Sym(), embedded)
	field.Embedded = 1
	outer.SetUnderlying(u.NewStruct([]*types.Field{field}))
	n := g.NewNameAt(pos, c.Lookup("outer"), outer)
	n.Class = ir.PEXTERN
	dot := c.Expr(c.AddImplicitDots(g.NewSelectorExpr(pos, ir.ODOT, n, member.Sym))).(*ir.SelectorExpr)
	if dot.Type() != member.Type || dot.Selection != member || dot.X.(*ir.SelectorExpr).Selection != field {
		t.Fatal("embedded selection rewrite")
	}
	c.CalcMethods(outer)
	if len(outer.AllMethods()) != 1 || outer.AllMethods()[0].Sym != method.Sym || outer.AllMethods()[0].Embedded != 1 {
		t.Fatal("promoted method set")
	}
}

func TestNativeTypeSwitchDiagnosticOwner(t *testing.T) {
	c := newTestContext(8)
	c.InitUniverse()
	g, u := c.IR, c.IR.Types
	pos := u.PosTable.XPos(src.MakePos(src.NewFileBase("switch.go", ""), 4, 2))
	value := g.NewNameAt(pos, c.Lookup("value"), u.Types[types.TINTER])
	value.Class = ir.PEXTERN
	cases := []*ir.CaseClause{
		g.NewCaseStmt(pos, []ir.Node{g.TypeNode(value.Type())}, nil),
		g.NewCaseStmt(pos, []ir.Node{g.TypeNode(value.Type())}, nil),
	}
	c.Stmt(g.NewSwitchStmt(pos, g.NewTypeSwitchGuard(pos, nil, value), cases))
	ds := g.Diagnostics.Diagnostics()
	if len(ds) != 1 || ds[0].Code != "DuplicateCase" || ds[0].Filename != "switch.go" || ds[0].Line != 4 {
		t.Fatalf("type switch owner: %v", ds)
	}
}

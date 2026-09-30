package pkginit

import (
	"bytes"
	"fmt"
	"go-backend-ssa/internal/compare"
	"go-backend-ssa/internal/ir"
	"go-backend-ssa/internal/types"
	"strings"
	"testing"
)

const compareSource = `package p
 type Mixed struct { S string; Bytes [4]byte; N int; I any; T string; More [4]byte; M int }
 type Nested struct { A [2]any; B struct{ X int; Y float64 } };func CompareNested(a,b Nested)bool{return a==b}
 type Padded struct { A byte; B int64; C byte }
 type Blanks struct { A int; _ any; B int }
 type Large struct { A [64]byte; B int };func CompareLarge(a,b Large)bool{return a==b}
 type Words struct { A,B,C,D,E,F int }
 type Halfwords struct { A,B,C,D uint16 }
 type Small struct { A,B int }
 type Float struct { A float64; B complex128 }
 type I interface { M() }
 type Text string
 func CompareMixed(a,b Mixed)bool{return a==b}
 func ComparePadded(a,b Padded)bool{return a==b}
 func CompareBlanks(a,b Blanks)bool{return a==b}
 func CompareWords(a,b Words)bool{return a==b}
 func CompareHalfwords(a,b Halfwords)bool{return a==b}
 func CompareSmall(a,b Small)bool{return a==b}
 func CompareFloat(a,b Float)bool{return a==b}
 func CompareString(a,b string)bool{return a==b}
 func CompareNamedString(a,b Text)bool{return a==b}
 func CompareEmpty(a,b any)bool{return a==b}
 func CompareIface(a,b I)bool{return a==b}
 func CompareLiteral(a string)bool{return a=="constant"}
`

func compareContext(t *testing.T) *compare.Context {
	p := sourceContext(t, "example/p", compareSource, false)
	arch := p.Noder.Typecheck.IR.Base.Ctxt.Arch
	arch.Alignment = 1
	arch.CanMergeLoads = true
	return compare.NewContext(p.Noder.Typecheck)
}
func compareExpr(c *compare.Context, name string) (*ir.Func, *ir.BinaryExpr) {
	for _, fn := range c.Typecheck.Target.Funcs {
		if fn.Sym().Name == name {
			return fn, fn.Body[0].(*ir.ReturnStmt).Results[0].(*ir.BinaryExpr)
		}
	}
	panic(name)
}
func compareConditions(c *compare.Context, name string) (out []ir.Node, panicable bool) {
	fn, e := compareExpr(c, name)
	c.Typecheck.IR.WithFunc(fn, func() {
		switch {
		case e.X.Type().IsStruct():
			out, panicable = c.EqStruct(e.X.Type(), e.X, e.Y)
		case e.X.Type().IsString():
			a, b := c.EqString(e.X, e.Y)
			out = []ir.Node{a, b}
		case e.X.Type().IsInterface():
			a, b := c.EqInterface(e.X, e.Y)
			out = []ir.Node{a, b}
		}
	})
	return
}
func compareDump(nodes []ir.Node) string {
	var b bytes.Buffer
	for _, n := range nodes {
		ir.FDump(&b, "cond", n)
	}
	return b.String()
}
func TestNativeCompareConstruction(t *testing.T) {
	c := compareContext(t)
	for _, entry := range []struct{ name, callee string }{
		{"CompareString", "memequal"}, {"CompareNamedString", "memequal"}, {"CompareLiteral", "memequal"}, {"CompareEmpty", "efaceeq"}, {"CompareIface", "ifaceeq"},
	} {
		nodes, _ := compareConditions(c, entry.name)
		call := nodes[1].(*ir.CallExpr)
		if len(nodes) != 2 || nodes[0].Op() != ir.OEQ || call.Fun.(*ir.Name).Sym().Name != entry.callee || len(call.Args) != 3 {
			t.Fatalf("%s: native comparison shape", entry.name)
		}
		for _, n := range nodes {
			if !c.Typecheck.IR.OwnsNode(n) || n.Typecheck() != 1 {
				t.Fatal("comparison result ownership/checking")
			}
		}
	}
	nodes, canPanic := compareConditions(c, "CompareMixed")
	if !canPanic {
		t.Fatal("interface panic boundary lost")
	}
	var rows []string
	for _, n := range nodes {
		rows = append(rows, fmt.Sprint(n))
	}
	t.Log(rows)
	// Calls may move to the end of each safe region, but cannot cross I's equality.
	boundary := -1
	for i, n := range nodes {
		if b, ok := n.(*ir.BinaryExpr); ok && b.X.Type().IsInterface() {
			boundary = i
		}
	}
	if boundary < 0 {
		t.Fatal("missing interface comparison")
	}
	for i, n := range nodes {
		row := fmt.Sprint(n)
		if strings.Contains(row, ".S") && i > boundary || strings.Contains(row, ".T") && i < boundary {
			t.Fatal("memory comparison reordered across panic boundary", rows)
		}
	}
	if _, p := compareConditions(c, "CompareNested"); !p {
		t.Fatal("nested array/interface panic boundary lost")
	}
	if _, p := compareConditions(c, "CompareBlanks"); p {
		t.Fatal("blank interface field introduces panic")
	}
	_, e := compareExpr(c, "CompareBlanks")
	if compare.EqCanPanic(e.X.Type()) {
		t.Fatal("blank field is observable")
	}
	if c.Typecheck.IR.CurFunc != nil {
		t.Fatal("comparison construction leaked current function")
	}
}
func TestNativeCompareTargetCosts(t *testing.T) {
	c := compareContext(t)
	_, e := compareExpr(c, "CompareHalfwords")
	typ := e.X.Type()
	c.Types.CalcSize(typ)
	if n, next := compare.Memrun(typ, 0); n != 8 || next != 4 {
		t.Fatal("unaligned merge", n, next)
	}
	if cost := compare.EqStructCost(typ); cost != 1 {
		t.Fatal("merged cost", cost)
	}
	c.Ctxt.Arch.Alignment = 4
	c.Ctxt.Arch.CanMergeLoads = false
	if n, next := compare.Memrun(typ, 0); n != 2 || next != 1 {
		t.Fatal("aligned merge", n, next)
	}
	if cost := compare.EqStructCost(typ); cost != 4 {
		t.Fatal("separate cost", cost)
	}
	_, e = compareExpr(c, "ComparePadded")
	if n, next := compare.Memrun(e.X.Type(), 0); n != 1 || next != 1 {
		t.Fatal("padding included in memory run", n, next)
	}
}
func TestNativeCompareIsolation(t *testing.T) {
	old := compareContext(t)
	nodes, _ := compareConditions(old, "CompareMixed")
	before := compareDump(nodes)
	for j := 0; j < 8; j++ {
		t.Run(fmt.Sprint(j), func(t *testing.T) {
			t.Parallel()
			c := compareContext(t)
			c.Ctxt.Arch.Alignment = 4
			c.Ctxt.Arch.CanMergeLoads = false
			compareConditions(c, "CompareMixed")
			if compareDump(nodes) != before || old.Ctxt.Arch.Alignment != 1 {
				t.Fatal("comparison graph/target leaked")
			}
		})
	}
}
func TestNativeCompareRejectsForeignInputs(t *testing.T) {
	a, b := compareContext(t), compareContext(t)
	_, x := compareExpr(a, "CompareString")
	_, y := compareExpr(b, "CompareString")
	before := compareDump([]ir.Node{x, y})
	defer func() {
		if recover() == nil {
			t.Error("mixed graph accepted")
		}
		if compareDump([]ir.Node{x, y}) != before {
			t.Error("changed operands before rejecting foreign graph")
		}
	}()
	a.EqString(x.X, y.Y)
}
func TestNativeCompareRegularMemoryWithoutLink(t *testing.T) {
	// Preserve the pre-existing type-only predicate's lack of an IR requirement.
	u := types.NewUniverse(8, 8, 1<<50)
	if !compare.IsRegularMemory(u.NewPtr(u.TypeMem)) {
		t.Fatal("pointer not AMEM")
	}
}

func TestNativeCompareLargeRunFailure(t *testing.T) {
	c := compareContext(t)
	defer func() {
		p := recover()
		if p == nil || !strings.Contains(fmt.Sprint(p), "SubstArgTypes: too many argument types") {
			t.Fatalf("expected pinned upstream limitation, got %v", p)
		}
	}()
	compareConditions(c, "CompareLarge")
}

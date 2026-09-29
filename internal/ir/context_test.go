package ir

import (
	"bytes"
	"fmt"
	"go-backend-ssa/internal/cmd/obj"
	"go-backend-ssa/internal/cmd/src"
	"go-backend-ssa/internal/types"
	"go/constant"
	"strings"
	"sync"
	"testing"
)

func graphContext(size int) *Context {
	link := &obj.Link{}
	u := types.NewUniverseWithPositions(size, size, 1<<50, &link.PosTable)
	u.LocalPkg = u.NewPkg("example/p", "p")
	c := NewContext(u, link)
	// The callback from native typecheck.InitUniverse, using actual ir.Name.
	u.InitTypes(func(s *types.Sym, t *types.Type) types.Object {
		n := c.NewDeclNameAt(src.XPos{}, OTYPE, s)
		n.SetType(t)
		n.SetTypecheck(1)
		s.Def = n
		return n
	})
	return c
}

func TestNativeGraphCopyTraversalAndTypes(t *testing.T) {
	c := graphContext(8)
	u := c.Types
	typ := u.Types[types.TINT]
	obj := typ.Obj().(*Name)
	if obj.Sym().Def != obj || c.TypeNode(typ) != obj || obj.Type() != typ {
		t.Fatal("builtin name/type identity broken")
	}
	p := c.Base.Ctxt.PosTable.XPos(src.MakePos(src.NewFileBase("one.go", "/src/one.go"), 10, 2))
	if u.PosTable.Pos(p).Filename() != "one.go" {
		t.Fatal("type and IR position namespaces differ")
	}
	x := c.NewNameAt(p, u.LocalPkg.Lookup("x"), typ)
	x.Class = PAUTO
	one := c.NewBasicLit(p, typ, constant.MakeInt64(1))
	add := c.NewBinaryExpr(p, OADD, x, one)
	add.SetType(typ)
	add.SetTypecheck(1)
	stmt := c.NewReturnStmt(p, []Node{add})
	copied := DeepCopy(src.XPos{}, stmt).(*ReturnStmt)
	if copied == stmt || copied.Results[0] == add {
		t.Fatal("deep copy did not copy structural nodes")
	}
	dup := copied.Results[0].(*BinaryExpr)
	if dup.X != x || dup.Y != one || nodeContext(dup) != c || nodeContext(copied) != c {
		t.Fatal("native shared leaves or ownership lost")
	}
	var ops []Op
	Visit(copied, func(n Node) { ops = append(ops, n.Op()) })
	if fmt.Sprint(ops) != "[return + NAME LITERAL]" {
		t.Fatalf("visit order: %v", ops)
	}
	replacement := c.NewBasicLit(p, typ, constant.MakeInt64(2))
	EditChildren(dup, func(n Node) Node {
		if n == one {
			return replacement
		}
		return n
	})
	if add.Y != one || dup.Y != replacement {
		t.Fatal("copy editing mutated original")
	}
	if got := fmt.Sprint(stmt); got != "return x + 1" {
		t.Fatalf("native format: %q", got)
	}
	var dump bytes.Buffer
	c.FDumpList(&dump, "body", Nodes{stmt})
	if !strings.Contains(dump.String(), "one.go:10:2") {
		t.Fatalf("source position missing: %s", dump.String())
	}
	if got := fmt.Sprintf("%v/%+v", Nodes(nil), Nodes(nil)); got != "/ <nil>" {
		t.Fatalf("nil node list formatting: %q", got)
	}
}

func TestNativeFunctionsRestoreContextAndClosures(t *testing.T) {
	c := graphContext(8)
	u := c.Types
	p := c.Base.Ctxt.PosTable.XPos(src.MakePos(src.NewFileBase("fn.go", "/src/fn.go"), 3, 1))
	sig := u.NewSignature(nil, []*types.Field{u.NewField(p, u.LocalPkg.Lookup("x"), u.Types[types.TINT])}, nil)
	fn := c.NewFunc(p, p, u.LocalPkg.Lookup("F"), sig)
	fn.DeclareParams(true)
	if fn.Nname.Func != fn || fn.Dcl[0].Curfn != fn || fn.Dcl[0].Class != PPARAM || sig.Param(0).Nname != fn.Dcl[0] {
		t.Fatal("native parameter links")
	}
	orig := c.Types.Pos
	func() {
		defer func() {
			if recover() != "stop" {
				t.Error("missing callback panic")
			}
		}()
		c.WithFunc(fn, func() {
			if c.CurFunc != fn || c.Types.Pos != p {
				t.Fatal("function scope not installed")
			}
			panic("stop")
		})
	}()
	if c.CurFunc != nil || c.Types.Pos != orig {
		t.Fatal("WithFunc failed to restore state")
	}
	var pkg Package
	closure := c.NewClosureFunc(p, p, OCLOSURE, u.NewSignature(nil, nil, nil), fn, &pkg, 1)
	capture := c.NewClosureVar(p, closure, fn.Dcl[0])
	if capture.Outer != fn.Dcl[0] || capture.Canonical() != fn.Dcl[0] || !capture.IsClosureVar() || closure.ClosureVars[0] != capture || pkg.Funcs[0] != closure || closure.ClosureParent != fn || closure.OClosure.Func != closure {
		t.Fatal("native closure graph identity")
	}
	if closure.Linksym().ABI() != obj.ABIInternal {
		t.Fatal("closure symbol ABI")
	}
	other := graphContext(8)
	func() {
		defer func() {
			if recover() == nil {
				t.Error("foreign Func accepted")
			}
		}()
		other.WithFunc(fn, func() { t.Error("foreign callback invoked") })
	}()
}

func TestIRInstancesConcurrentAndRetained(t *testing.T) {
	const count = 12
	graphs := make([]*BinaryExpr, count)
	contexts := make([]*Context, count)
	var wg sync.WaitGroup
	for i := range count {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c := graphContext(4 + 4*(i%2))
			contexts[i] = c
			p := c.Base.Ctxt.PosTable.XPos(src.MakePos(src.NewFileBase(fmt.Sprintf("p%d.go", i), fmt.Sprintf("/p%d.go", i)), uint(i+1), 1))
			n := c.NewNameAt(p, c.Types.LocalPkg.Lookup("x"), c.Types.Types[types.TINT])
			n.Class = PAUTO
			expr := c.NewBinaryExpr(p, OADD, n, c.NewInt(p, int64(i)))
			graphs[i] = expr
			c.EscFmt = func(Node) string { return fmt.Sprintf("instance%d", i) }
			c.IsIntrinsicCall = func(*CallExpr) bool { return i%2 == 0 }
		}()
	}
	wg.Wait()
	for i, g := range graphs {
		c := contexts[i]
		if nodeContext(g) != c || g.X.Type().Size() != int64(4+4*(i%2)) {
			t.Fatal("instance layout lost")
		}
		got := fmt.Sprintf("%+v", g)
		if !strings.Contains(got, fmt.Sprintf("instance%d", i)) || !strings.Contains(got, fmt.Sprintf("p%d.go", i)) {
			t.Fatalf("retained graph rebound: %s", got)
		}
		if i > 0 && (g.X.Sym() == graphs[0].X.Sym() || g.X.Type() == graphs[0].X.Type() || c.Base == contexts[0].Base) {
			t.Fatal("instances share mutable compiler objects")
		}
	}
}

func TestIRRejectsUnsharedPositionTable(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("unrelated native position tables accepted")
		}
	}()
	NewContext(types.NewUniverse(8, 8, 1<<50), &obj.Link{})
}

func TestIRPreservesPreexistingPositions(t *testing.T) {
	link := &obj.Link{}
	pos := link.PosTable.XPos(src.MakePos(src.NewFileBase("existing.go", "/src/existing.go"), 21, 4))
	u := types.NewUniverseWithPositions(8, 8, 1<<50, &link.PosTable)
	c := NewContext(u, link)
	n := c.NewNameAt(pos, u.LocalPkg.Lookup("x"), nil)
	if c.Line(n) != "existing.go:21:4" || u.PosTable.Pos(pos).Filename() != "existing.go" || link.PosTable.Pos(u.AutogeneratedPos).Filename() != "<autogenerated>" {
		t.Fatal("existing XPos was rebound")
	}
}

package typecheck

import (
	"fmt"
	"go-backend-ssa/internal/cmd/obj"
	"go-backend-ssa/internal/cmd/src"
	"go-backend-ssa/internal/ir"
	"go-backend-ssa/internal/types"
	"go/constant"
	"sync"
	"testing"
)

func newTestContext(size int) *Context {
	link := &obj.Link{}
	u := types.NewUniverseWithPositions(size, size, 1<<50, &link.PosTable)
	u.LocalPkg = u.NewPkg("example/p", "p")
	return NewContext(ir.NewContext(u, link))
}
func TestNativeUniverseBindings(t *testing.T) {
	c := newTestContext(8)
	c.InitUniverse()
	u := c.IR.Types
	for _, entry := range c.builtinFuncs {
		n := u.BuiltinPkg.Lookup(entry.name).Def.(*ir.Name)
		if n.BuiltinOp != entry.op || n.Typecheck() != 1 {
			t.Fatalf("builtin %s", entry.name)
		}
	}
	for _, entry := range c.unsafeFuncs {
		n := u.UnsafePkg.Lookup(entry.name).Def.(*ir.Name)
		if n.BuiltinOp != entry.op || n.Typecheck() != 1 {
			t.Fatalf("unsafe builtin %s", entry.name)
		}
	}
	for _, kind := range []types.Kind{types.TINT, types.TSTRING, types.TFLOAT64, types.TBOOL} {
		typ := u.Types[kind]
		n := typ.Obj().(*ir.Name)
		if n.Type() != typ || n.Sym().Def != n || n.Op() != ir.OTYPE || c.IR.TypeNode(typ) != n {
			t.Fatalf("name/type binding: %v", kind)
		}
	}
	for _, name := range []string{"true", "false"} {
		n := u.BuiltinPkg.Lookup(name).Def.(*ir.Name)
		if n.Type() != u.UntypedBool || constant.BoolVal(n.Val()) != (name == "true") {
			t.Fatal(name)
		}
	}
	blank := c.IR.BlankNode
	if u.BlankSym != c.Lookup("_") || blank != u.BlankSym.Def || blank.Sym() != u.BlankSym || blank.Type() != u.Types[types.TBLANK] {
		t.Fatal("local blank binding")
	}
	builtinBlank := u.BuiltinPkg.Lookup("_").Def.(*ir.Name)
	if builtinBlank == blank || builtinBlank.Sym() == blank.Sym() {
		t.Fatal("builtin and package blank collapsed")
	}
	nilNode := u.BuiltinPkg.Lookup("nil").Def.(ir.Node)
	if nilNode.Op() != ir.ONIL || nilNode.Type() != u.Types[types.TNIL] {
		t.Fatal("nil binding")
	}
	if !u.IsOrdered[types.TSTRING] || !c.IR.OKForConst[types.TSTRING] || !u.IsSimple[types.TINT] || !c.okfor[ir.OADD][types.TSTRING] || c.okfor[ir.OMUL][types.TSTRING] {
		t.Fatal("native operation tables")
	}
	// These slices must reference this instance's arrays, as in the compiler.
	c.okforadd[types.TINT] = false
	if c.okfor[ir.OADD][types.TINT] || !c.okfor[ir.OSUB][types.TINT] {
		t.Fatal("native table aliases lost")
	}
	before := u.Types[types.TINT]
	func() {
		defer func() {
			if recover() == nil {
				t.Error("reinitialization accepted")
			}
		}()
		c.InitUniverse()
	}()
	if u.Types[types.TINT] != before || before.Obj().Sym().Def != before.Obj() {
		t.Fatal("failed reinitialization damaged live graph")
	}
}

func TestSharedFunctionPositionAndParameterHelpers(t *testing.T) {
	c := newTestContext(8)
	c.InitUniverse()
	u := c.IR.Types
	pos := u.PosTable.XPos(src.MakePos(src.NewFileBase("f.go", "/src/f.go"), 9, 2))
	sig := u.NewSignature(nil, nil, nil)
	f := c.IR.NewFunc(pos, pos, c.Lookup("F"), sig)
	other := c.IR.NewFunc(pos, pos, c.Lookup("G"), sig)
	c.IR.WithFunc(f, func() {
		if c.NodNil().Pos() != pos {
			t.Fatal("typecheck missed IR current position")
		}
		if c.AutoLabel(".loop").Name != ".loop0" || c.AutoLabel(".loop").Name != ".loop1" {
			t.Fatal("function label counter")
		}
		c.IR.WithFunc(other, func() {
			if c.AutoLabel(".loop").Name != ".loop0" {
				t.Fatal("nested function label counter")
			}
		})
		if c.IR.CurFunc != f || c.AutoLabel(".loop").Name != ".loop2" {
			t.Fatal("current function not restored")
		}
	})
	if u.Pos != (src.XPos{}) || c.IR.CurFunc != nil {
		t.Fatal("shared scope state not restored")
	}
	field := u.NewField(pos, c.Lookup("args"), u.NewSlice(u.Types[types.TINT]))
	field.SetIsDDD(true)
	params := c.NewFuncParams([]*types.Field{field})
	if len(params) != 1 || params[0] == field || params[0].Type != field.Type || params[0].Sym != field.Sym || params[0].Pos != pos || !params[0].IsDDD() {
		t.Fatal("native parameter clone")
	}
	func() {
		defer func() {
			if recover() == nil {
				t.Error("AutoLabel outside function accepted")
			}
		}()
		c.AutoLabel(".loop")
	}()
}

func TestConcurrentUniversesAndRetainedBindings(t *testing.T) {
	const count = 16
	contexts := make([]*Context, count)
	var wg sync.WaitGroup
	for i := range count {
		wg.Add(1)
		go func() { defer wg.Done(); c := newTestContext(4 + 4*(i%2)); c.InitUniverse(); contexts[i] = c }()
	}
	wg.Wait()
	for i, c := range contexts {
		u := c.IR.Types
		typ := u.Types[types.TINT]
		if typ.Size() != int64(4+4*(i%2)) || c.IR.TypeNode(typ) != typ.Obj() {
			t.Fatal("retained type binding")
		}
		if i > 0 {
			first := contexts[0]
			if typ == first.IR.Types.Types[types.TINT] || c.IR.BlankNode == first.IR.BlankNode || c.Lookup("_") == first.Lookup("_") || &c.okfor[ir.OADD][0] == &first.okfor[ir.OADD][0] {
				t.Fatal("shared mutable universe")
			}
		}
		if fmt.Sprint(u.BuiltinPkg.Lookup("true").Def) != "true" {
			t.Fatal("retained constant")
		}
	}
	contexts[0].IR.Types.IsOrdered[types.TINT] = false
	contexts[0].okforadd[types.TINT] = false
	for _, c := range contexts[1:] {
		if !c.okfor[ir.OLT][types.TINT] || !c.okfor[ir.OADD][types.TINT] {
			t.Fatal("operation table mutation escaped instance")
		}
	}
}

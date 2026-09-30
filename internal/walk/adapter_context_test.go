package walk

import (
	"fmt"
	"go-backend-ssa/internal/cmd/obj"
	"go-backend-ssa/internal/cmd/sys"
	"go-backend-ssa/internal/escape"
	"go-backend-ssa/internal/ir"
	"go-backend-ssa/internal/noder"
	"go-backend-ssa/internal/objw"
	"go-backend-ssa/internal/reflectdata"
	"go-backend-ssa/internal/rttype"
	"go-backend-ssa/internal/staticdata"
	"go-backend-ssa/internal/staticinit"
	"go-backend-ssa/internal/typecheck"
	"go-backend-ssa/internal/types"
	"go-backend-ssa/internal/types2"
	"testing"
)

func targetContext(size int, family sys.ArchFamily) *Context {
	link := &obj.Link{Arch: &obj.LinkArch{Arch: &sys.Arch{PtrSize: size, RegSize: size, Family: family}}}
	u := types.NewUniverseWithPositions(size, size, 1<<50, &link.PosTable)
	u.LocalPkg = u.NewPkg("p", "p")
	tc := typecheck.NewContext(ir.NewContext(u, link))
	tc.InitUniverse()
	return &Context{Typecheck: tc}
}
func TestNativeIndexCost(t *testing.T) {
	for _, family := range []sys.ArchFamily{sys.AMD64, sys.ARM, sys.ARM64, sys.I386, sys.Loong64, sys.MIPS, sys.MIPS64, sys.PPC64, sys.RISCV64, sys.S390X, sys.Wasm} {
		c := targetContext(8, family)
		for _, width := range []int64{0, 1, 2, 3, 4, 8, 16} {
			want := false
			switch family {
			case sys.AMD64, sys.ARM, sys.ARM64, sys.I386:
				want = width == 1 || width == 2 || width == 4 || width == 8
			case sys.Loong64, sys.PPC64, sys.S390X:
				want = width == 1
			}
			if got := c.cheapComputableIndex(width); got != want {
				t.Errorf("family=%d width=%d got=%t", family, width, got)
			}
		}
	}
}
func TestNativeMayCall(t *testing.T) {
	c := targetContext(8, sys.AMD64)
	g := c.Typecheck.IR
	u := g.Types
	name := func(s string, t *types.Type) *ir.Name {
		n := g.NewNameAt(u.Pos, u.LocalPkg.Lookup(s), t)
		n.Class = ir.PPARAM
		n.SetTypecheck(1)
		return n
	}
	i := name("i", u.Types[types.TINT])
	f := name("f", u.Types[types.TFLOAT64])
	z := name("z", u.Types[types.TCOMPLEX128])
	b := name("b", u.Types[types.TBOOL])
	bin := func(op ir.Op, x ir.Node, t *types.Type) *ir.BinaryExpr {
		n := g.NewBinaryExpr(u.Pos, op, x, x)
		n.SetType(t)
		return n
	}
	for _, soft := range []bool{false, true} {
		c.SoftFloat = soft
		for _, test := range []struct {
			n    ir.Node
			want bool
		}{
			{i, false}, {g.NewInt(u.Pos, 1), false}, {bin(ir.OADD, i, i.Type()), false}, {bin(ir.OADD, f, f.Type()), soft}, {bin(ir.OADD, z, z.Type()), soft}, {bin(ir.OEQ, f, b.Type()), soft}, {g.NewConvExpr(u.Pos, ir.OCONV, i.Type(), f), soft}, {g.NewConvExpr(u.Pos, ir.OCONV, u.Types[types.TINT64], i), false}, {bin(ir.ODIV, i, i.Type()), true}, {g.NewLogicalExpr(u.Pos, ir.OANDAND, b, b), false},
		} {
			if got := c.mayCall(test.n); got != test.want {
				t.Errorf("soft=%t node=%v got=%t want=%t", soft, test.n, got, test.want)
			}
		}
	}
	right := name("right", b.Type())
	right.PtrInit().Append(g.NewAssignStmt(u.Pos, i, i))
	if !c.mayCall(g.NewLogicalExpr(u.Pos, ir.OOROR, b, right)) {
		t.Fatal("conditional init hidden")
	}
	g.Base.Flag.Cfg.Instrumenting = true
	if !c.mayCall(i) || c.mayCall(g.NewInt(u.Pos, 1)) {
		t.Fatal("instrumentation or literal ordering")
	}
}
func TestWalkTargetIsolation(t *testing.T) {
	old := targetContext(4, sys.MIPS)
	g := old.Typecheck.IR
	u := g.Types
	f := g.NewNameAt(u.Pos, u.LocalPkg.Lookup("f"), u.Types[types.TFLOAT64])
	f.Class = ir.PPARAM
	n := g.NewBinaryExpr(u.Pos, ir.OADD, f, f)
	n.SetType(f.Type())
	for i := 0; i < 8; i++ {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			t.Parallel()
			c := targetContext(8, sys.AMD64)
			c.SoftFloat = true
			if !c.cheapComputableIndex(8) || old.cheapComputableIndex(8) || old.mayCall(n) {
				t.Fatal("target leaked across compilations")
			}
		})
	}
}

func TestWalkClearsFunctionCaches(t *testing.T) {
	tc := targetContext(8, sys.AMD64).Typecheck
	tc.IR.Base.Ctxt.Arch.Arch = sys.NewContext().ArchAMD64
	nd, err := noder.NewContext(types2.NewContext(), tc)
	if err != nil {
		t.Fatal(err)
	}
	writer := objw.NewContext(tc.IR.Types, tc.IR.Base.Ctxt, "linux")
	rt := rttype.NewContext(writer)
	rt.Init()
	nd.ReflectData = reflectdata.NewContext(tc, rt, staticdata.NewContext(tc.IR, writer))
	nd.StaticInit = staticinit.NewContext(tc, nd.ReflectData)
	c := NewPackageState(tc)
	c.BindFrontend(nd, escape.NewContext(tc, nd.Inline.Log))
	g := c.Typecheck.IR
	u := g.Types
	for _, name := range []string{"first", "second"} {
		fn := g.NewFunc(u.Pos, u.Pos, u.LocalPkg.Lookup(name), u.NewSignature(nil, nil, nil))
		c.staticValues = map[ir.Node]ir.Node{fn: fn}
		c.shapeConvSources = map[*ir.Name]*types.Type{fn.Nname: fn.Type()}
		c.Walk(fn)
		if c.staticValues != nil || c.shapeConvSources != nil || g.CurFunc != fn {
			t.Fatal("walk retained analysis caches or changed the driver contract")
		}
	}
}

package amd64

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"go-backend-ssa/internal/abi"
	"go-backend-ssa/internal/cmd/obj"
	"go-backend-ssa/internal/cmd/obj/x86"
	"go-backend-ssa/internal/cmd/src"
	"go-backend-ssa/internal/cmd/sys"
	"go-backend-ssa/internal/ir"
	"go-backend-ssa/internal/liveness"
	"go-backend-ssa/internal/logopt"
	"go-backend-ssa/internal/objw"
	"go-backend-ssa/internal/reflectdata"
	"go-backend-ssa/internal/rttype"
	"go-backend-ssa/internal/ssa"
	"go-backend-ssa/internal/ssagen"
	"go-backend-ssa/internal/staticdata"
	"go-backend-ssa/internal/typecheck"
	"go-backend-ssa/internal/types"
)

func testBackend(locations bool) *ssagen.Context {
	a := sys.NewContext().ArchAMD64
	link := &obj.Link{Arch: &obj.LinkArch{Arch: a}, Pkgpath: "example/p", Pathname: "/src", Flag_locationlists: locations}
	if err := link.Headtype.Set("linux"); err != nil {
		panic(err)
	}
	u := types.NewUniverseWithPositions(8, 8, 1<<50, &link.PosTable)
	u.LocalPkg = u.NewPkg("example/p", "p")
	u.Experiment.RegabiArgs = true
	u.Experiment.Dwarf5 = true
	g := ir.NewContext(u, link)
	tc := typecheck.NewContext(g)
	tc.InitUniverse()
	tc.InitRuntime()
	g.Base.Flag.LowerC = 1
	g.Base.Flag.WB = true
	g.Base.Flag.Dwarf = true
	rt := rttype.NewContext(objw.NewContext(u, link, "linux"))
	rt.Init()
	c := ssagen.NewContext(tc)
	c.SSA = ssa.NewContext(abi.NewContext(g), reflectdata.NewContext(tc, rt, staticdata.NewContext(g, rt.Writer)), logopt.NewContext())
	c.SSA.GOAMD64 = 1
	c.SSA.FramePointerEnabled = true
	c.SSA.Now = time.Now
	c.Liveness = liveness.NewContext(c.SSA)
	NewContext(c)
	c.InitConfig()
	c.InitTables()
	if err := c.SSA.PhaseOption("check", "on", 1, ""); err != "" {
		panic(err)
	}
	return c
}
func testFunction(c *ssagen.Context, name string) *ir.Func {
	g, u := c.Typecheck.IR, c.Typecheck.IR.Types
	p := g.Base.Ctxt.PosTable.XPos(src.MakePos(src.NewFileBase("emission.go", "/src/emission.go"), 11, 1))
	fn := g.NewFunc(p, p, u.LocalPkg.Lookup(name), u.NewSignature(nil, nil, nil))
	fn.DeclareParams(true)
	g.InitLSym(fn, true)
	fn.Body = []ir.Node{g.NewReturnStmt(p, nil)}
	return fn
}
func instructions(pp *objw.Progs) string {
	var b strings.Builder
	for p := pp.Text; p != nil; p = p.Link {
		fmt.Fprintln(&b, p.String())
	}
	return b.String()
}
func TestNativeInstructionContexts(t *testing.T) {
	for i := range 8 {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			t.Parallel()
			a, b := testBackend(true), testBackend(false)
			if a.Arch.LinkArch.DWARFRegisters[x86.REG_DI] != 5 {
				t.Fatal("native DWARF register map missing")
			}
			b.Arch.LinkArch.DWARFRegisters[x86.REG_DI] = 99
			if a.Arch.LinkArch.DWARFRegisters[x86.REG_DI] != 5 {
				t.Fatal("DWARF maps shared")
			}
			fn := testFunction(a, "empty")
			f := a.BuildSSA(fn, 0, false)
			w := a.SSA.ReflectData.Runtime.Writer
			pp := w.NewProgs(fn, 0)
			a.GenSSA(f, pp)
			got := instructions(pp)
			if !strings.Contains(got, "TEXT") || !strings.Contains(got, "RET") || strings.Contains(got, "A???") || fn.LSym.Func().GCArgs == nil || fn.LSym.Func().GCLocals == nil {
				t.Fatalf("incomplete native emission\n%s", got)
			}
			other := testFunction(b, "other")
			g := b.BuildSSA(other, 0, false)
			qw := b.SSA.ReflectData.Runtime.Writer
			qq := qw.NewProgs(other, 0)
			b.GenSSA(g, qq)
			if instructions(pp) != got {
				t.Fatal("another context changed retained instruction buffer")
			}
			qq.Free()
			if qw.OwnsProgs(qq) {
				t.Fatal("freed buffer remains active")
			}
			pp.Free()
			if w.OwnsProgs(pp) {
				t.Fatal("freed buffer remains active")
			}
		})
	}
}
func TestInstructionOwnerChecksBeforeWrites(t *testing.T) {
	a, b := testBackend(false), testBackend(false)
	fn := testFunction(a, "owned")
	f := a.BuildSSA(fn, 0, false)
	pp := a.SSA.ReflectData.Runtime.Writer.NewProgs(fn, 0)
	before := instructions(pp)
	rejects := func(label string, run func()) {
		t.Helper()
		func() {
			defer func() {
				if recover() == nil {
					t.Error(label, "accepted")
				}
			}()
			run()
		}()
		if instructions(pp) != before {
			t.Fatal(label, "changed instructions before rejecting")
		}
	}
	rejects("foreign backend", func() { b.GenSSA(f, pp) })
	other := testFunction(b, "foreign")
	qq := b.SSA.ReflectData.Runtime.Writer.NewProgs(other, 0)
	rejects("foreign buffer", func() { a.GenSSA(f, qq) })
	rejects("foreign liveness buffer", func() { a.Liveness.ArgLiveness(fn, f, qq) })
	rejects("foreign liveness function", func() { a.Liveness.Compute(other, f, 0, pp, false) })
	ownedDebug := a.SSA.BindFuncDebug(ssa.FuncDebug{})
	rejects("foreign debug link", func() { ssa.BuildFuncDebug(b.Typecheck.IR.Base.Ctxt, f, 0, a.StackOffset, &ownedDebug) })
	foreignDebug := b.SSA.BindFuncDebug(ssa.FuncDebug{})
	rejects("foreign debug result", func() { ssa.BuildFuncDebug(a.Typecheck.IR.Base.Ctxt, f, 0, a.StackOffset, &foreignDebug) })
	rejects("invalid worker", func() { a.SSA.ReflectData.Runtime.Writer.NewProgs(fn, 1) })
	rejects("foreign function", func() { a.SSA.ReflectData.Runtime.Writer.NewProgs(other, 0) })
	old := a.Arch.SSAGenValue
	a.Arch.SSAGenValue = nil
	rejects("missing handler", func() { a.GenSSA(f, pp) })
	a.Arch.SSAGenValue = old
	a.GenSSA(f, pp)
	pp.Free()
	qq.Free()
}

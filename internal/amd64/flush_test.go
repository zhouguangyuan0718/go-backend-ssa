package amd64

import (
	"bytes"
	"flag"
	"fmt"
	"testing"

	"go-backend-ssa/internal/cmd/obj"
	"go-backend-ssa/internal/dwarfgen"
	"go-backend-ssa/internal/objw"
	"go-backend-ssa/internal/ssagen"
)

func flushFunction(c *ssagen.Context, name string) (*obj.LSym, *objw.Progs) {
	fn := testFunction(c, name)
	f := c.BuildSSA(fn, 0, false)
	pp := c.SSA.ReflectData.Runtime.Writer.NewProgs(fn, 0)
	c.GenSSA(f, pp)
	pp.Flush()
	return fn.LSym, pp
}
func TestNativeFlushContexts(t *testing.T) {
	for i := range 8 {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			t.Parallel()
			a, b := testBackend(true), testBackend(false)
			b.Typecheck.IR.Types.Experiment.Dwarf5 = false
			dwarfgen.NewContext(a, a.SSA.ReflectData)
			dwarfgen.NewContext(b, b.SSA.ReflectData)
			s, pp := flushFunction(a, "full")
			if len(s.P) == 0 || s.Func().Pcln.Pcsp == nil || s.Func().Pcln.Pcline == nil {
				t.Fatal("missing assembled text or PC tables")
			}
			saved := bytes.Clone(s.P)
			pc := bytes.Clone(s.Func().Pcln.Pcline.P)
			abbr := bytes.Clone(a.Typecheck.IR.Base.Ctxt.DWARF.GetAbbrev())
			q, qq := flushFunction(b, "other")
			if len(q.P) == 0 {
				t.Fatal("second flush empty")
			}
			_ = b.Typecheck.IR.Base.Ctxt.DWARF.GetAbbrev()
			if !bytes.Equal(saved, s.P) || !bytes.Equal(pc, s.Func().Pcln.Pcline.P) || !bytes.Equal(abbr, a.Typecheck.IR.Base.Ctxt.DWARF.GetAbbrev()) {
				t.Fatal("cross-context mutation")
			}
			qq.Free()
			pp.Free()
		})
	}
}
func TestFlushOwnerBeforeWrites(t *testing.T) {
	a, b := testBackend(true), testBackend(true)
	da := dwarfgen.NewContext(a, a.SSA.ReflectData)
	dwarfgen.NewContext(b, b.SSA.ReflectData)
	fn := testFunction(a, "owned")
	f := a.BuildSSA(fn, 0, false)
	pp := a.SSA.ReflectData.Runtime.Writer.NewProgs(fn, 0)
	a.GenSSA(f, pp)
	before := instructions(pp)
	reject := func(name string, run func()) {
		t.Helper()
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("%s accepted", name)
				}
			}()
			run()
		}()
		if instructions(pp) != before || len(fn.LSym.P) != 0 {
			t.Fatalf("%s changed native instructions", name)
		}
	}
	foreign := testFunction(b, "foreign")
	reject("foreign compiler function", func() { old := pp.CurFunc; pp.CurFunc = foreign; defer func() { pp.CurFunc = old }(); pp.Flush() })
	reject("foreign program", func() {
		old := pp.Text.Ctxt
		pp.Text.Ctxt = b.Typecheck.IR.Base.Ctxt
		defer func() { pp.Text.Ctxt = old }()
		pp.Flush()
	})
	reject("foreign list tail", func() {
		tail := pp.Text.Link
		old := tail.Ctxt
		tail.Ctxt = b.Typecheck.IR.Base.Ctxt
		defer func() { tail.Ctxt = old }()
		pp.Flush()
	})
	reject("foreign debug function", func() { da.Info(a.Typecheck.IR.Base.Ctxt, fn.LSym, &obj.LSym{}, foreign) })
	reject("foreign debug link", func() { da.Info(b.Typecheck.IR.Base.Ctxt, fn.LSym, &obj.LSym{}, fn) })
	pp.Flush()
	pp.Free()
	func() {
		defer func() {
			if recover() == nil {
				t.Error("freed buffer accepted")
			}
		}()
		pp.Flush()
	}()
}

func TestDwarfFlagSets(t *testing.T) {
	a, b := testBackend(true), testBackend(true)
	da, db := dwarfgen.NewContext(a, a.SSA.ReflectData), dwarfgen.NewContext(b, b.SSA.ReflectData)
	da.Flags = flag.NewFlagSet("a", flag.ContinueOnError)
	db.Flags = flag.NewFlagSet("b", flag.ContinueOnError)
	da.Flags.(*flag.FlagSet).String("mode", "default", "")
	db.Flags.(*flag.FlagSet).String("mode", "default", "")
	if err := da.Flags.(*flag.FlagSet).Set("mode", "left"); err != nil {
		t.Fatal(err)
	}
	if err := db.Flags.(*flag.FlagSet).Set("mode", "right"); err != nil {
		t.Fatal(err)
	}
	da.RecordFlags("mode")
	db.RecordFlags("mode")
	la, lb := a.Typecheck.IR.Base.Ctxt, b.Typecheck.IR.Base.Ctxt
	if len(la.Data) != 1 || len(lb.Data) != 1 || string(la.Data[0].P) != "-mode=left regabi" || string(lb.Data[0].P) != "-mode=right regabi" {
		t.Fatal("producer flag contexts crossed")
	}
	da.RecordPackageName()
	db.RecordPackageName()
	if string(la.Data[1].P) != "p" || string(lb.Data[1].P) != "p" {
		t.Fatal("package name missing")
	}
}

func TestDwarfContextBindings(t *testing.T) {
	a, b := testBackend(true), testBackend(false)
	link := a.Typecheck.IR.Base.Ctxt
	rejects := func(run func()) {
		t.Helper()
		func() {
			defer func() {
				if recover() == nil {
					t.Error("foreign debug services accepted")
				}
			}()
			run()
		}()
		if link.DWARF != nil || link.DebugInfo != nil || link.GenAbstractFunc != nil {
			t.Fatal("failed binding modified Link")
		}
	}
	rejects(func() { dwarfgen.NewContext(a, b.SSA.ReflectData) })
	ownSSA := a.SSA
	rejects(func() { a.SSA = b.SSA; defer func() { a.SSA = ownSSA }(); dwarfgen.NewContext(a, ownSSA.ReflectData) })

	b.Typecheck.IR.Base.Ctxt.EnsureDwarfFixups()
	rejects(func() {
		link.DwFixups = b.Typecheck.IR.Base.Ctxt.DwFixups
		defer func() { link.DwFixups = nil }()
		dwarfgen.NewContext(a, ownSSA.ReflectData)
	})
	dwarfgen.NewContext(a, ownSSA.ReflectData)
}

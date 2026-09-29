package ir

import (
	"encoding/binary"
	"sync"
	"testing"

	"go-backend-ssa/internal/cmd/obj"
	"go-backend-ssa/internal/cmd/objabi"
	"go-backend-ssa/internal/cmd/src"
	"go-backend-ssa/internal/cmd/sys"
	"go-backend-ssa/internal/go/abi"
)

func TestNativeFunctionSymbolsAreInstanceOwned(t *testing.T) {
	const count = 8
	contexts := make([]*Context, count)
	funcs := make([]*Func, count)
	var wg sync.WaitGroup
	for i := range contexts {
		wg.Go(func() {
			c := graphContext(8)
			contexts[i] = c
			link := c.Base.Ctxt
			link.Arch = &obj.LinkArch{Arch: &sys.Arch{Name: "amd64", PtrSize: 8, RegSize: 8, ByteOrder: binary.LittleEndian}}
			link.TargetOS = "linux"
			link.Pkgpath = "example/p"
			link.DiagFunc = func(s string, args ...any) { t.Errorf(s, args...) }
			link.DwFixups = obj.NewDwarfFixupTable(link)
			pos := link.PosTable.XPos(src.MakePos(src.NewFileBase("symbols.go", "/src/symbols.go"), 17, 1))
			f := c.NewFunc(pos, pos, c.Types.LocalPkg.Lookup("F"), c.Types.NewSignature(nil, nil, nil))
			f.SetWrapper(true)
			f.SetNeedctxt(true)
			f.SetDupok(true)
			f.Pragma = Nosplit | Systemstack
			c.InitLSym(f, true)
			funcs[i] = f
			if f.LSym != f.Nname.LinksymABI(f.ABI) || len(link.Text) != 1 || link.Text[0] != f.LSym || f.LSym.Type != objabi.STEXT || f.LSym.Func().StartLine != 17 || f.LSym.Func().FuncID != abi.FuncIDWrapper {
				t.Error("native text symbol identity/metadata changed")
			}
			want := obj.AttrWrapper | obj.AttrNeedCtxt | obj.AttrDuplicateOK | obj.AttrNoSplit | obj.AttrCFunc
			if f.LSym.Attribute&want != want {
				t.Error("function flags missing")
			}
			link.DwFixups.SetPrecursorFunc(f.LSym, f)
			if link.DwFixups.GetPrecursorFunc(f.LSym) != f {
				t.Error("precursor lost")
			}
			abs := link.DwFixups.AbsFuncDwarfSym(f.LSym)
			if abs.Name != "go:info.example/p.F$abstract" || abs.Type != objabi.SDWARFABSFCN || !abs.DuplicateOK() {
				t.Error("abstract symbol metadata changed")
			}
			f2 := c.NewFunc(pos, pos, c.Types.LocalPkg.Lookup("External"), c.Types.NewSignature(nil, nil, nil))
			c.InitLSym(f2, false)
			if f2.LSym == nil || f2.LSym.Func() != nil || len(link.Text) != 1 {
				t.Error("bodyless function emitted as text")
			}
		})
	}
	wg.Wait()
	for i, c := range contexts {
		for j, d := range contexts {
			if i != j && (funcs[i].LSym == funcs[j].LSym || c.Base.Ctxt.DwFixups == d.Base.Ctxt.DwFixups) {
				t.Fatal("instances share symbols or fixups")
			}
		}
		if c.Base.Ctxt.DwFixups.GetPrecursorFunc(funcs[i].LSym) != funcs[i] {
			t.Fatal("old graph not retained")
		}
	}
	c := contexts[0]
	assertPanic := func(f func()) {
		t.Helper()
		defer func() {
			if recover() == nil {
				t.Error("expected panic")
			}
		}()
		f()
	}
	assertPanic(func() { c.InitLSym(funcs[0], true) })
	foreign := contexts[1].NewFunc(src.XPos{}, src.XPos{}, contexts[1].Types.LocalPkg.Lookup("Foreign"), contexts[1].Types.NewSignature(nil, nil, nil))
	assertPanic(func() { c.InitLSym(foreign, true) })
	if foreign.LSym != nil {
		t.Fatal("foreign graph mutated before rejection")
	}
}

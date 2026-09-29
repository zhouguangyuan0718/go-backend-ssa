package obj

import (
	"encoding/binary"
	"sync"
	"testing"

	"go-backend-ssa/internal/cmd/dwarf"
	"go-backend-ssa/internal/cmd/objabi"
	"go-backend-ssa/internal/cmd/src"
	"go-backend-ssa/internal/cmd/sys"
)

func TestNativeTextDwarfAndFixups(t *testing.T) {
	for _, locations := range []bool{false, true} {
		for _, early := range []bool{false, true} {
			link := &Link{Arch: &LinkArch{Arch: &sys.Arch{Name: "amd64", PtrSize: 8, RegSize: 8, ByteOrder: binary.LittleEndian}}, TargetOS: "linux", Flag_locationlists: locations, statichash: make(map[string]*LSym)}
			link.DiagFunc = func(f string, a ...any) { t.Errorf(f, a...) }
			link.DwFixups = NewDwarfFixupTable(link)
			sym := link.LookupStatic("p.F")
			if early {
				link.DwFixups.SetPrecursorFunc(sym, nil)
				sym.Set(AttrWasInlined, true)
			}
			link.InitTextSym(sym, 0, src.XPos{})
			if !early {
				link.DwFixups.SetPrecursorFunc(sym, nil)
				sym.Set(AttrWasInlined, true)
			}
			info, loc, ranges, abs, lines := link.dwarfSym(sym)
			if info.Type != objabi.SDWARFFCN || ranges.Type != objabi.SDWARFRANGE || lines.Type != objabi.SDWARFLINES || abs.Type != objabi.SDWARFABSFCN || (loc != nil) != locations {
				t.Fatal("native DWARF symbol initialization")
			}
			if !abs.Static() || abs != link.LookupStatic("go:info.p.F$abstract") || abs == link.Lookup(abs.Name) {
				t.Fatal("derived static namespace lost")
			}
			if locations && loc.Type != objabi.SDWARFLOC {
				t.Fatal("location-list kind")
			}
			// References may be recorded by independent backend workers on one table.
			refs := make([]*LSym, 16)
			var wg sync.WaitGroup
			for i := range refs {
				refs[i] = &LSym{R: []Reloc{{Add: int64(i)}}}
				wg.Go(func() { link.DwFixups.ReferenceChildDIE(refs[i], 0, abs, 4, 0) })
			}
			wg.Wait()
			var calls int
			link.GenAbstractFunc = func(s *LSym) {
				calls++
				if s != sym {
					t.Error("wrong precursor")
				}
				link.DwFixups.RegisterChildDIEOffsets(abs, []*dwarf.Var{{ChildIndex: 4}}, []int32{23})
			}
			link.DwFixups.Finalize("p", false)
			for i, r := range refs {
				if r.R[0].Add != int64(i)+23 {
					t.Fatal("delayed relocation fixup", i, r.R)
				}
			}
			if calls != 1 {
				t.Fatal("missing abstract function was not generated once")
			}
			immediate := &LSym{R: []Reloc{{Add: 7}}}
			link.DwFixups.ReferenceChildDIE(immediate, 0, abs, 4, 0)
			if immediate.R[0].Add != 30 {
				t.Fatal("immediate relocation fixup")
			}
			info2, loc2, ranges2, abs2, lines2 := link.dwarfSym(sym)
			if info != info2 || loc != loc2 || ranges != ranges2 || abs != abs2 || lines != lines2 {
				t.Fatal("DWARF symbols not cached")
			}
		}
	}
}

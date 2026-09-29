package objw

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"go-backend-ssa/internal/bitvec"
	"go-backend-ssa/internal/cmd/obj"
	"go-backend-ssa/internal/cmd/objabi"
	"go-backend-ssa/internal/cmd/sys"
	"go-backend-ssa/internal/types"
	"sync"
	"testing"
)

func testWriter(size int, order binary.ByteOrder, goos, arch string) *Context {
	link := &obj.Link{Arch: &obj.LinkArch{Arch: &sys.Arch{Name: arch, PtrSize: size, RegSize: size, ByteOrder: order}}}
	u := types.NewUniverseWithPositions(size, size, 1<<50, &link.PosTable)
	return NewContext(u, link, goos)
}
func TestNativeObjectDataAndRelocations(t *testing.T) {
	for _, size := range []int{4, 8} {
		for _, order := range []binary.ByteOrder{binary.LittleEndian, binary.BigEndian} {
			c := testWriter(size, order, "linux", "test")
			link := c.Link
			s, target := link.Lookup("p.data"), link.Lookup("p.target")
			c.Uint8(s, 0, 0xaa)
			c.Bool(s, 1, true)
			c.Uint16(s, 2, 0x1122)
			c.Uint32(s, 4, 0x33445566)
			c.Uintptr(s, 8, 0x778899)
			if s.P[0] != 0xaa || s.P[1] != 1 || order.Uint16(s.P[2:]) != 0x1122 || order.Uint32(s.P[4:]) != 0x33445566 {
				t.Fatal("target byte order")
			}
			off := c.SymPtr(s, 8+size+1, target, 13)
			if off != 8+3*size || len(s.R) != 1 || s.R[0].Off != int32(8+2*size) || s.R[0].Siz != uint8(size) || s.R[0].Type != objabi.R_ADDR || s.R[0].Sym != target || s.R[0].Add != 13 {
				t.Fatal("pointer alignment/relocation", off, s.R)
			}
			off = c.SymPtrWeak(s, off, target, 17)
			off = c.SymPtrOff(s, off, target)
			off = c.SymPtrWeakOff(s, off, target)
			if s.R[1].Type != objabi.R_WEAKADDR || s.R[2].Type != objabi.R_ADDROFF || s.R[3].Type != objabi.R_WEAKADDROFF {
				t.Fatal("relocation kind")
			}
			start := off
			off = c.Uvarint(s, off, 300)
			if !bytes.Equal(s.P[start:off], []byte{0xac, 2}) {
				t.Fatal("varint")
			}
			off = c.BitVec(s, off, bitvec.BitVec{N: 16, B: []uint32{0x1234}})
			if !bytes.Equal(s.P[off-2:off], []byte{0x34, 0x12}) {
				t.Fatal("bitmap byte order")
			}
			c.Global(s, int32(off), obj.RODATA|obj.DUPOK|obj.LOCAL)
			if s.Type != objabi.SRODATA || s.Size != int64(off) || len(link.Data) != 1 || link.Data[0] != s || !s.OnList() {
				t.Fatal("global declaration")
			}
			var diags []string
			link.DiagFunc = func(f string, a ...any) { diags = append(diags, fmt.Sprintf(f, a...)) }
			c.Global(s, int32(off), obj.RODATA)
			if link.Errors != 1 || len(diags) != 1 {
				t.Fatal("native duplicate diagnostic")
			}
		}
	}
}
func TestNativeFIPSStateAndRetainedData(t *testing.T) {
	var wg sync.WaitGroup
	for i := range 12 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c := testWriter(8, binary.LittleEndian, "linux", "amd64")
			link := c.Link
			link.Pkgpath = "crypto/internal/fips140/sha256"
			link.Flag_shared = true
			var trace bytes.Buffer
			link.TraceWriter = &trace
			if i%2 == 0 {
				link.SetFIPSDebugHash("qn")
			} else {
				link.SetFIPSDebugHash("vy")
			}
			s := link.Lookup("crypto/internal/fips140/sha256.data")
			c.Uint32(s, 0, 7)
			want := objabi.SDATA
			if i%2 != 0 {
				want = objabi.SDATAFIPS
			}
			if s.Type != want {
				t.Errorf("FIPS state: %v", s.Type)
			}
			var diags []string
			link.DiagFunc = func(f string, a ...any) { diags = append(diags, fmt.Sprintf(f, a...)) }
			c.SymPtr(s, 8, link.Lookup("p.target"), 0)
			if (i%2 != 0) != (len(diags) == 1) || (i%2 != 0) != (trace.Len() > 0) {
				t.Error("FIPS diagnostics isolation")
			}
			old := append([]byte(nil), s.P...)
			other := testWriter(8, binary.LittleEndian, "linux", "amd64")
			other.Uint32(other.Link.Lookup(s.Name), 0, 99)
			if !bytes.Equal(s.P, old) {
				t.Error("old data changed")
			}
		}()
	}
	wg.Wait()
	for _, target := range []struct {
		os, arch string
		size     int
	}{{"windows", "386", 4}, {"js", "wasm", 8}, {"aix", "ppc64", 8}} {
		c := testWriter(target.size, binary.BigEndian, target.os, target.arch)
		s := c.Link.Lookup("crypto/internal/fips140/pkg.data")
		c.Uint32(s, 0, 3)
		if s.Type != objabi.SDATA {
			t.Fatal("native excluded FIPS target", target)
		}
	}
}
func TestObjectTargetCannotBeRebound(t *testing.T) {
	c := testWriter(8, binary.LittleEndian, "linux", "amd64")
	defer func() {
		if recover() == nil {
			t.Fatal("target OS rebinding accepted")
		}
	}()
	NewContext(c.Types, c.Link, "windows")
}

func TestNativeWriteFailuresPreservePriorState(t *testing.T) {
	c := testWriter(8, binary.LittleEndian, "linux", "amd64")
	c.Link.SetFIPSDebugHash("qn")
	func() {
		defer func() {
			if recover() == nil {
				t.Error("invalid native bisect pattern accepted")
			}
		}()
		c.Link.SetFIPSDebugHash("!")
	}()
	s := c.Link.Lookup("crypto/internal/fips140/pkg.value")
	c.Uint32(s, 0, 9)
	before := append([]byte(nil), s.P...)
	func() {
		defer func() {
			if recover() == nil {
				t.Error("misaligned integer accepted")
			}
		}()
		c.Uint16(s, 1, 1)
	}()
	if !bytes.Equal(before, s.P) || s.Type != objabi.SDATA {
		t.Fatal("failure mutated bytes or matcher")
	}
}

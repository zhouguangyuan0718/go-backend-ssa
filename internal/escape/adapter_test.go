package escape

import (
	"fmt"
	"go-backend-ssa/internal/cmd/obj"
	"go-backend-ssa/internal/ir"
	"go-backend-ssa/internal/logopt"
	"go-backend-ssa/internal/typecheck"
	"go-backend-ssa/internal/types"
	"strings"
	"testing"
)

func summaryContext() *Context {
	link := &obj.Link{}
	u := types.NewUniverseWithPositions(8, 8, 1<<50, &link.PosTable)
	tc := typecheck.NewContext(ir.NewContext(u, link))
	tc.InitUniverse()
	return NewContext(tc, logopt.NewContext())
}
func TestNativeEscapeSummaryCache(t *testing.T) {
	c := summaryContext()
	var l leaks
	l.AddHeap(c, 4)
	l.AddMutator(c, 1)
	l.AddCallee(c, 3)
	l.AddResult(c, 0, 2)
	l.AddResult(c, 1, 6)
	l.Optimize(c)
	encoded := l.Encode(c)
	decoded := c.ParseLeaks(encoded)
	if decoded != l || l.Heap() != 4 || l.Mutator() != 1 || l.Callee() != 3 || l.Result(0) != 2 || l.Result(1) != -1 {
		t.Fatal("native summary roundtrip", l, decoded)
	}
	if len(c.leakTagCache) != 1 || c.leakTagCache[l] != encoded {
		t.Fatal("summary cache not owned")
	}
	for _, legacy := range []string{"", "legacy"} {
		if c.ParseLeaks(legacy).Heap() != 0 {
			t.Fatal("legacy conservative summary lost")
		}
	}
	if empty := c.ParseLeaks("esc:"); empty.Any() {
		t.Fatal("empty summary escapes")
	}
	var saturated leaks
	saturated.AddHeap(c, 1000)
	if saturated.Heap() != 254 {
		t.Fatal("native saturation lost")
	}
	for i := 0; i < 8; i++ {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			t.Parallel()
			other := summaryContext()
			if len(other.leakTagCache) != 0 {
				t.Fatal("shared cache")
			}
			copy := other.ParseLeaks(encoded)
			if copy.Encode(other) != encoded || len(other.leakTagCache) != 1 || len(c.leakTagCache) != 1 || c.leakTagCache[l] != encoded {
				t.Fatal("summary retention failed")
			}
		})
	}
}
func TestNativeEscapeSummaryInvalidDepth(t *testing.T) {
	c := summaryContext()
	defer func() {
		v := recover()
		if v == nil || !strings.Contains(fmt.Sprint(v), "invalid derefs count") {
			t.Errorf("invalid depth: %v", v)
		}
	}()
	var l leaks
	l.AddHeap(c, -2)
}
func TestNativeHeapAllocationReasonInputs(t *testing.T) {
	a, b := summaryContext(), summaryContext()
	if a.HeapAllocReason(nil) != "" {
		t.Fatal("native nil input changed")
	}
	foreign := b.Typecheck.IR.NewInt(b.Typecheck.IR.Types.Pos, 1)
	defer func() {
		if recover() == nil {
			t.Error("foreign heap allocation input accepted")
		}
	}()
	a.HeapAllocReason(foreign)
}

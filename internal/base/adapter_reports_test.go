package base

import (
	"bytes"
	"fmt"
	"strings"
	"sync"
	"testing"

	"go-backend-ssa/internal/cmd/obj"
	"go-backend-ssa/internal/cmd/src"
)

func TestConcurrentNativeErrorReports(t *testing.T) {
	var output bytes.Buffer
	c := NewLinkContext(&obj.Link{TraceWriter: &output})
	c.Flag.LowerE = 1
	file := src.NewFileBase("parallel.go", "/parallel.go")
	positions := make([]src.XPos, 32)
	for i := range positions {
		positions[i] = c.Ctxt.PosTable.XPos(src.MakePos(file, uint(i+1), 3))
	}
	var workers sync.WaitGroup
	for i, pos := range positions {
		workers.Go(func() { c.ReportError(pos, 0, "problem %d", i) })
	}
	workers.Wait()
	c.FlushReports()
	var want strings.Builder
	for i := range positions {
		fmt.Fprintf(&want, "parallel.go:%d:3: problem %d\n", i+1, i)
	}
	if output.String() != want.String() || c.Errors() != 32 {
		t.Fatalf("lost, unordered, or duplicated reports: count=%d\n%s", c.Errors(), output.String())
	}
	c.FlushReports()
	if output.String() != want.String() {
		t.Fatal("flushing repeated diagnostics")
	}
	other := NewLinkContext(&obj.Link{})
	if other.Errors() != 0 || other.SyntaxErrors() != 0 {
		t.Fatal("error state leaked")
	}
}

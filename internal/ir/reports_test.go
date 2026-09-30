package ir

import (
	"bytes"
	"go-backend-ssa/internal/cmd/src"
	"strings"
	"testing"
)

func TestGenericDumpNilAndPositions(t *testing.T) {
	c := graphContext(8)
	for _, root := range []any{nil, (*Func)(nil), (*Name)(nil), Nodes(nil)} {
		var out bytes.Buffer
		c.FDumpAny(&out, root, "", 4)
		if !strings.Contains(out.String(), "nil") {
			t.Fatalf("nil dump: %q", out.String())
		}
	}
	pos := c.Base.Ctxt.PosTable.XPos(src.MakePos(src.NewFileBase("report.go", "/src/report.go"), 7, 3))
	var out bytes.Buffer
	c.FDumpAny(&out, struct{ Where src.XPos }{pos}, "", 4)
	if !strings.Contains(out.String(), c.Base.FmtPos(pos)) {
		t.Fatal("position belongs to wrong table", out.String())
	}
}

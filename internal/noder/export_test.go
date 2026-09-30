package noder

import (
	"bytes"
	"errors"
	"fmt"
	"go-backend-ssa/internal/ir"
	"go-backend-ssa/internal/types"
	"io"
	"testing"
)

func exportContext(t *testing.T, answer int) *Context {
	t.Helper()
	c := bodyContext(t)
	c.Flag.LowerL = 1
	c.ReadLocalPackage(localStub(t, c, fmt.Sprintf(`package p;const Answer=%d;func Small(x int)int{return x+Answer};type Box[T any]struct{V T}`, answer)))
	c.Inline.CanInlineFuncs(c.Typecheck.Target.Funcs, nil)
	return c
}
func TestNativeUnifiedExportRoundtrip(t *testing.T) {
	for i := range 8 {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			t.Parallel()
			a, b := exportContext(t, i+1), exportContext(t, i+101)
			var outputs [2]bytes.Buffer
			for j, c := range []*Context{a, b} {
				c.ReflectData.Runtime.Writer.NumberSymbols()
				if err := c.ExportUnified(&outputs[j]); err != nil {
					t.Fatal(err)
				}
			}
			saved := bytes.Clone(outputs[0].Bytes())
			if bytes.Equal(saved, outputs[1].Bytes()) {
				t.Fatal("distinct exports shared")
			}
			for j, data := range []*bytes.Buffer{&outputs[0], &outputs[1]} {
				imported := bodyContext(t)
				imported.Types.LocalPkg = imported.Types.NewPkg("example/consumer", "consumer")
				pkg := imported.Types.NewPkg("example/p", "p")
				imported.RegisterPackage(pkg, data.String(), false)
				pri := imported.objReader[pkg.Lookup("Answer")]
				constant := pri.pr.objIdx(pri.idx, nil, nil, false).(*ir.Name)
				want := int64(i + 1 + j*100)
				if got := imported.Typecheck.IR.Int64Val(constant); got != want {
					t.Fatal(got, want)
				}
				fn, err := imported.LookupFunc("example/p.Small")
				if err != nil || fn.Inl == nil || !imported.unifiedHaveInlineBody(fn) {
					t.Fatal("inline body export missing", err, fn)
				}
				bp := imported.objReader[pkg.Lookup("Box")]
				box := bp.pr.objIdx(bp.idx, nil, []*types.Type{imported.Types.Types[types.TINT]}, false).(*ir.Name)
				if box.Type().Kind() != types.TSTRUCT || box.Type().Field(0).Type != imported.Types.Types[types.TINT] {
					t.Fatal("generic type export lost")
				}
			}
			var unused bytes.Buffer
			func() {
				defer func() {
					if recover() == nil {
						t.Error("repeat export accepted")
					}
				}()
				_ = a.ExportUnified(&unused)
			}()
			if unused.Len() != 0 || !bytes.Equal(saved, outputs[0].Bytes()) {
				t.Fatal("repeat export touched output")
			}
		})
	}
}

type failedExport struct{ err error }

func (w failedExport) Write([]byte) (int, error) { return 0, w.err }
func TestUnifiedExportBoundary(t *testing.T) {
	c := exportContext(t, 42)
	var out bytes.Buffer
	func() {
		defer func() {
			if recover() == nil {
				t.Error("unnumbered export accepted")
			}
		}()
		_ = c.ExportUnified(&out)
	}()
	if out.Len() != 0 || !c.ExportAvailable() {
		t.Fatal("failed export preflight mutated state")
	}
	c.ReflectData.Runtime.Writer.NumberSymbols()
	failure := errors.New("explicit export writer failure")
	if err := c.ExportUnified(failedExport{failure}); !errors.Is(err, failure) {
		t.Fatal(err)
	}
	if c.ExportAvailable() {
		t.Fatal("failed export remained reusable")
	}
	next := exportContext(t, 43)
	next.ReflectData.Runtime.Writer.NumberSymbols()
	if err := next.ExportUnified(io.Discard); err != nil {
		t.Fatal(err)
	}
}

package amd64

import (
	"context"
	"encoding/binary"
	"errors"
	"go-backend-ssa/internal/gc"
	"go-backend-ssa/internal/ir"
	"testing"
)

func TestPackageConstructionAndDeferSlots(t *testing.T) {
	c, s := loadQueueCompiler(t, 1, `package p
var M=map[int]int{1:2}
func F(p *int){defer func(){*p+=1}();defer func(){*p+=2}()}
func G(n int)func()int{return func()int{return n+M[1]}}
`, true)
	c.PrepareFrontend(s, nil)
	built, err := c.BuildDeclarations(s, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(built) < 5 || c.QueuedFunctions() != 0 || c.Writer.Link.InParallel || c.Writer.Types.CalcSizeDisabled {
		t.Fatal("incomplete declaration fixed point", len(built))
	}
	var checked bool
	for _, held := range built {
		f := held.Func()
		if ir.FuncName(f.Frontend().Func()) != "F" {
			continue
		}
		d := held.OpenDefers()
		if d.Bits == nil || len(d.Closures) != 2 {
			t.Fatal("missing deferred slots")
		}
		first := d.Closures[0]
		d.Closures[0] = nil
		if held.OpenDefers().Closures[0] != first {
			t.Fatal("defer list aliases mutable adapter storage")
		}
		held.ContinueNative()
		data := f.Frontend().Func().LSym.Func().OpenCodedDeferInfo.P
		bits, n := binary.Uvarint(data)
		slot, m := binary.Uvarint(data[n:])
		if n <= 0 || m <= 0 || bits != uint64(-d.Bits.FrameOffset()) || slot != uint64(-first.FrameOffset()) {
			t.Fatal("slot identities do not describe native defer metadata", data, bits, slot)
		}
		checked = true
	}
	if !checked {
		t.Fatal("missing defer function")
	}
	mustDataPanic(t, func() { c.FinishPackage() })
	c.DumpData()
	mustDataPanic(t, func() {
		c.SetObjectHeader(gc.ObjectConfig{GOOS: "linux", GOARCH: "amd64", GOAMD64: 1, Version: "go1.27.1"})
	})
}
func TestPackageConstructionCancellationJoinsQueue(t *testing.T) {
	c, s := loadQueueCompiler(t, 1, "package p;func F()int{return 1};func G()int{return 2};func H()int{return 3}", true)
	c.PrepareFrontend(s, nil)
	calls := 0
	got, err := c.BuildDeclarations(s, nil, func() error {
		calls++
		if calls == 2 {
			return context.Canceled
		}
		return nil
	})
	if got != nil || !errors.Is(err, context.Canceled) || calls != 2 || c.QueuedFunctions() != 0 || c.Writer.Link.InParallel || c.Writer.Types.CalcSizeDisabled {
		t.Fatal("cancelled queue did not join", got, err, calls)
	}
	mustDataPanic(t, func() { c.BuildDeclarations(s, nil, nil) })
	mustDataPanic(t, func() { c.FinishPackage() })
	other, syms := loadQueueCompiler(t, 1, "package p;func Good()int{return 9}", true)
	other.PrepareFrontend(syms, nil)
	if _, err := other.BuildDeclarations(syms, nil, nil); err != nil {
		t.Fatal(err)
	}
}

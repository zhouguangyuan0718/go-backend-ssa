package amd64

import (
	"bytes"
	"fmt"
	"go-backend-ssa/internal/cmd/obj"
	"go-backend-ssa/internal/cmd/src"
	"go-backend-ssa/internal/gc"
	"go-backend-ssa/internal/ir"
	"go-backend-ssa/internal/ssa"
	"go-backend-ssa/internal/ssagen"
	"go-backend-ssa/internal/types"
	"strings"
	"testing"
)

const fixedPointSource = `package p
type Node struct { Name string; Next *Node }
func(n Node) Size()int{return len(n.Name)}
type Box struct{ Values [3]Node }
var Global Box
const Answer int = %d
func Value(x int)int{return x+Answer}
func Equal(a,b Box)bool{return a==b}
func Convert(n Node)any{return n}
func Method(n Node)func()int{return n.Size}
`

func TestNativeDeclarationFixedPoint(t *testing.T) {
	for i := range 8 {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			t.Parallel()
			c, s := queueCompiler(t, 4, fmt.Sprintf(fixedPointSource, i+1))
			c.RaceEnabled = i%2 == 0
			initial := len(c.Package.Noder.Typecheck.Target.Funcs)
			c.CompileDeclarations(s, nil)
			c.Backend.CheckLargeStacks()
			if c.QueuedFunctions() != 0 || c.Writer.Link.InParallel || c.Writer.Types.CalcSizeDisabled || c.Writer.Link.DwFixups != nil || c.Package.Noder.Flag.GenDwarfInl != 0 || c.Package.Noder.Diagnostics.HasErrors() {
				t.Fatal("fixed point state", c.Package.Noder.Diagnostics.Diagnostics())
			}
			if len(c.Package.Noder.Typecheck.Target.Funcs) <= initial {
				t.Fatal("runtime type helpers were not generated")
			}
			helpers := 0
			for _, f := range c.Package.Noder.Typecheck.Target.Funcs {
				if len(f.Body) == 0 || f.IsClosure() {
					continue
				}
				if f.LSym == nil || len(f.LSym.P) == 0 {
					t.Fatal("uncompiled generated function", f)
				}
				if strings.Contains(f.LSym.Name, "type:.eq.") {
					helpers++
				}
			}
			if helpers == 0 {
				t.Fatal("native equality helpers missing")
			}
			var descriptor *obj.LSym
			for _, n := range c.Package.Noder.Typecheck.Target.Externs {
				if n.Op() == ir.OTYPE && n.Sym().Name == "Node" {
					descriptor = c.Package.Noder.ReflectData.TypeLinksym(n.Type())
				}
			}
			if descriptor == nil || !descriptor.OnList() || len(descriptor.P) == 0 {
				t.Fatal("runtime type descriptor missing")
			}
			globals := c.Writer.Link.Lookup("example/p.Global")
			if !globals.OnList() || globals.Size == 0 {
				t.Fatal("global storage missing")
			}
			c.DumpData()
			c.SetObjectHeader(gc.ObjectConfig{GOOS: "linux", GOARCH: "amd64", GOAMD64: 1, Version: "go1.27.1"})
			var out objectOutput
			if e := c.WriteArchive(&out, gc.CombinedObject); e != nil {
				t.Fatal(e)
			}
			saved := bytes.Clone(out.data)
			other, syms := queueCompiler(t, 2, fmt.Sprintf(fixedPointSource, i+101))
			other.CompileDeclarations(syms, nil)
			other.DumpData()
			other.SetObjectHeader(gc.ObjectConfig{GOOS: "linux", GOARCH: "amd64", GOAMD64: 1, Version: "go1.27.1"})
			var next objectOutput
			if e := other.WriteArchive(&next, gc.CombinedObject); e != nil {
				t.Fatal(e)
			}
			if !bytes.Equal(saved, out.data) || bytes.Equal(out.data, next.data) {
				t.Fatal("retained package changed")
			}
			mustDataPanic(t, func() { c.CompileDeclarations(s, nil) })
			mustDataPanic(t, func() { c.CompileQueued(nil) })
		})
	}
}
func TestDeclarationFixedPointPreflight(t *testing.T) {
	c, s := queueCompiler(t, 2, "package p;func Value(x int)int{return x+1}")
	other, foreign := queueCompiler(t, 2, "package p;func Value(x int)int{return x+2}")
	mustDataPanic(t, func() { c.CompileDeclarations(foreign, nil) })
	if c.QueuedFunctions() != 0 {
		t.Fatal("failed preflight queued work")
	}
	c.CompileDeclarations(s, nil)
	mustDataPanic(t, func() { c.EnqueueFunc(c.Package.Noder.Typecheck.Target.Funcs[0], s) })
	other.EnqueueFunc(other.Package.Noder.Typecheck.Target.Funcs[0], foreign)
	mustDataPanic(t, func() { other.CompileDeclarations(foreign, nil) })
	other.CompileQueued(nil)
}
func TestDeclarationFixedPointWorkerFailure(t *testing.T) {
	c, s := queueCompiler(t, 4, fmt.Sprintf(fixedPointSource, 1))
	marker := new(int)
	c.Backend.Arch.SSAGenValue = func(*ssagen.State, *ssa.Value) { panic(marker) }
	func() {
		defer func() {
			if failure := recover(); failure != marker {
				t.Errorf("worker failure identity: %v", failure)
			}
		}()
		c.CompileDeclarations(s, nil)
	}()
	if c.Writer.Link.InParallel || c.Writer.Types.CalcSizeDisabled {
		t.Fatal("worker phase flags retained")
	}
	mustDataPanic(t, func() { c.DumpData() })
	mustDataPanic(t, func() { c.AddGCLocals() })
	mustDataPanic(t, func() { c.CompileDeclarations(s, nil) })
	next, syms := queueCompiler(t, 2, fmt.Sprintf(fixedPointSource, 2))
	next.CompileDeclarations(syms, nil)
	next.DumpData()
}

func TestDeclarationFixedPointAfterDwarf(t *testing.T) {
	c, s := queueCompiler(t, 2, "package p;func Value(x int)int{return x+1}")
	u := c.Writer.Types
	late := u.NewStruct([]*types.Field{u.NewField(src.XPos{}, u.LocalPkg.Lookup("Words"), u.NewArray(u.Types[types.TSTRING], 3))})
	link := c.Writer.Link
	fixups := link.DwFixups
	abstract := link.Lookup("example/p.lateAbstract")
	fixups.SetPrecursorFunc(abstract, nil)
	native := link.GenAbstractFunc
	calls := 0
	link.GenAbstractFunc = func(sym *obj.LSym) {
		if sym != abstract {
			native(sym)
			return
		}
		calls++
		if link.InParallel {
			t.Error("DWARF callback ran with workers")
		}
		c.Package.Noder.ReflectData.NeedRuntimeType(late)
		fixups.RegisterChildDIEOffsets(fixups.AbsFuncDwarfSym(sym), nil, nil)
	}
	before := len(c.Package.Noder.Typecheck.Target.Funcs)
	c.CompileDeclarations(s, nil)
	descriptor := c.Package.Noder.ReflectData.TypeLinksym(late)
	if calls != 1 || link.DwFixups != nil || !descriptor.OnList() || len(descriptor.P) == 0 || len(c.Package.Noder.Typecheck.Target.Funcs) <= before {
		t.Fatal("late DWARF type did not reach fixed point", calls, descriptor.Name)
	}
	for _, fn := range c.Package.Noder.Typecheck.Target.Funcs[before:] {
		if len(fn.Body) > 0 && (fn.LSym == nil || len(fn.LSym.P) == 0) {
			t.Fatal("late type helper was not compiled", fn)
		}
	}
	c.DumpData()
}

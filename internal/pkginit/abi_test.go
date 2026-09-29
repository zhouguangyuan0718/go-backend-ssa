package pkginit

import (
	"bytes"
	"fmt"
	"go-backend-ssa/internal/cmd/obj"
	"go-backend-ssa/internal/ir"
	"go-backend-ssa/internal/noder"
	"go-backend-ssa/internal/ssagen"
	"strings"
	"testing"
)

const abiSource = `package p
func Sum(a,b int)int{return a+b}
func Empty(){}
func Variadic(xs ...int)int{return len(xs)}
func Asm(a int)int
func AsmEmpty()
func Export(a int)int{return a}
func Unsafe(a int)int{return a}
func Linked(a int)int{return a}
`

func abiContext(t *testing.T) (*Context, *ssagen.Context, *ssagen.SymABIs) {
	c := sourceContext(t, "example/p", abiSource, false, func(n *noder.Context) { n.Flag.Complete = false })
	c.MakeTask()
	c.Noder.Types.Experiment.RegabiWrappers = true
	backend := ssagen.NewContext(c.Noder.Typecheck)
	return c, backend, backend.NewSymABIs()
}
func findABI(t *testing.T, c *Context, name string, abi obj.ABI, wrapper bool) *ir.Func {
	t.Helper()
	for _, fn := range c.Noder.Typecheck.Target.Funcs {
		if fn.Sym().Name == name && fn.ABI == abi && fn.ABIWrapper() == wrapper {
			return fn
		}
	}
	t.Fatalf("missing %s ABI %v wrapper %v", name, abi, wrapper)
	return nil
}
func readABIs(backend *ssagen.Context, s *ssagen.SymABIs, data string) {
	backend.ReadFile = func(name string) ([]byte, error) {
		if name != "input.symabis" {
			panic(name)
		}
		return []byte(data), nil
	}
	s.ReadSymABIs("input.symabis")
}
func TestNativeABIWrappers(t *testing.T) {
	c, b, s := abiContext(t)
	readABIs(b, s, "# native assembler information\n\ndef example/p.Asm ABI0\ndef example/p.AsmEmpty ABI0\nref example/p.Sum ABI0\nref example/p.Empty ABI0\nref example/p.Variadic ABI0\n")
	if !s.HasDef(c.Noder.Types.LocalPkg.Lookup("Asm")) || s.HasDef(c.Noder.Types.LocalPkg.Lookup("Sum")) || c.Noder.Ctxt.DwTextCount != 2 {
		t.Fatal("assembler definitions")
	}
	before := len(c.Noder.Typecheck.Target.Funcs)
	savedPos := c.Noder.Types.Pos
	s.GenABIWrappers()
	if len(c.Noder.Typecheck.Target.Funcs) != before+5 {
		t.Fatal("wrapper count")
	}
	for _, name := range []string{"Sum", "Empty", "Variadic"} {
		fn := findABI(t, c, name, obj.ABI0, true)
		if !fn.Dupok() || fn.Pragma&ir.Nosplit == 0 || fn.Nname.Defn != fn {
			t.Fatal("wrapper metadata", fn)
		}
	}
	if findABI(t, c, "Empty", obj.ABI0, true).Body[0].Op() != ir.OTAILCALL {
		t.Fatal("empty ABI0 wrapper not a tail call")
	}
	if findABI(t, c, "AsmEmpty", obj.ABIInternal, true).Body[0].Op() == ir.OTAILCALL {
		t.Fatal("amd64 internal-to-ABI0 tail call")
	}
	call := findABI(t, c, "Variadic", obj.ABI0, true).Body[0].(*ir.ReturnStmt).Results[0].(*ir.CallExpr)
	if !call.IsDDD || len(call.Args) != 1 {
		t.Fatal("variadic argument forwarding")
	}
	if c.Noder.Typecheck.IR.CurFunc != nil || c.Noder.Types.Pos != savedPos {
		t.Fatal("wrapper leaked current state")
	}
}
func TestNativeABIAttributesAndCgo(t *testing.T) {
	c, b, s := abiContext(t)
	f := findABI(t, c, "Unsafe", obj.ABIInternal, false)
	f.Pragma |= ir.CgoUnsafeArgs
	f.Sym().Linksym().Set(obj.AttrLinkname, true)
	f.Sym().Linksym().Set(obj.AttrLinknameStd, true)
	linked := findABI(t, c, "Linked", obj.ABIInternal, false)
	linked.Sym().Linkname = "external.Linked"
	c.Noder.Typecheck.Target.CgoPragmas = [][]string{{"cgo_export_static", "example/p.Export"}, {"cgo_export_dynamic", "example/p.Export", "external_export"}}
	readABIs(b, s, "")
	s.GenABIWrappers()
	if f.ABI != obj.ABI0 || !f.Sym().LinksymABI(obj.ABI0).IsLinknameStd() {
		t.Fatal("cgo unsafe definition ABI")
	}
	wrapper := findABI(t, c, "Unsafe", obj.ABIInternal, true)
	if !wrapper.LinksymABI(wrapper.ABI).IsLinkname() || !wrapper.LinksymABI(wrapper.ABI).IsLinknameStd() {
		t.Fatal("linkname propagation")
	}
	findABI(t, c, "Linked", obj.ABI0, true)
	if got := fmt.Sprint(c.Noder.Typecheck.Target.CgoPragmas); got != "[[cgo_export_static example/p.Export example/p.Export ABIInternal] [cgo_export_dynamic example/p.Export external_export ABIInternal]]" {
		t.Fatal(got)
	}
}
func TestNativeABIWithoutRegisterWrappers(t *testing.T) {
	c, b, s := abiContext(t)
	c.Noder.Types.Experiment.RegabiWrappers = false
	readABIs(b, s, "def example/p.Asm ABI0\nref example/p.Sum ABI0")
	n := len(c.Noder.Typecheck.Target.Funcs)
	s.GenABIWrappers()
	if len(c.Noder.Typecheck.Target.Funcs) != n || findABI(t, c, "Asm", obj.ABI0, false).ABIRefs == 0 {
		t.Fatal("definition/reference application without wrappers")
	}
}
func TestNativeABIInputFailures(t *testing.T) {
	for _, input := range []string{"def only", "def p.F ABI99", "wat p.F ABI0", `ref "".F ABI0`} {
		t.Run(input, func(t *testing.T) {
			_, b, s := abiContext(t)
			defer func() {
				if recover() == nil {
					t.Error("invalid input accepted")
				}
			}()
			readABIs(b, s, input)
		})
	}
	c, b, s := abiContext(t)
	b.ReadFile = func(string) ([]byte, error) { return nil, fmt.Errorf("input unavailable") }
	func() {
		defer func() {
			if x := recover(); x == nil || !strings.Contains(fmt.Sprint(x), "input unavailable") {
				t.Error(x)
			}
		}()
		s.ReadSymABIs("name")
	}()
	if c.Noder.Ctxt.DwTextCount != 0 {
		t.Fatal("failed input changed definitions")
	}
}
func TestNativeABIConflictDiagnostic(t *testing.T) {
	c, b, s := abiContext(t)
	readABIs(b, s, "def example/p.Sum ABI0")
	s.GenABIWrappers()
	diagnostics := c.Noder.Typecheck.IR.Diagnostics.Diagnostics()
	if len(diagnostics) != 1 || !strings.Contains(diagnostics[0].Message, "defined in both Go and assembly") {
		t.Fatal(diagnostics)
	}
}
func TestNativeABIIsolation(t *testing.T) {
	old, b, s := abiContext(t)
	readABIs(b, s, "ref example/p.Sum ABI0")
	s.GenABIWrappers()
	fn := findABI(t, old, "Sum", obj.ABI0, true)
	body := fmt.Sprint(fn.Body)
	for i := 0; i < 8; i++ {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			t.Parallel()
			c, b, s := abiContext(t)
			var trace bytes.Buffer
			c.Noder.Typecheck.IR.TraceWriter = &trace
			c.Noder.Debug.ABIWrap = 1
			readABIs(b, s, "ref example/p.Sum ABI0")
			s.GenABIWrappers()
			if findABI(t, c, "Sum", obj.ABI0, true) == fn || fmt.Sprint(fn.Body) != body || !strings.Contains(trace.String(), "wrapper for") {
				t.Fatal("ABI instances interfered")
			}
		})
	}
}
func TestNativeABIRejectsForeignTarget(t *testing.T) {
	a, _, s := abiContext(t)
	b, _, _ := abiContext(t)
	foreign := b.Noder.Typecheck.Target.Funcs[0]
	a.Noder.Typecheck.Target.Funcs = append(a.Noder.Typecheck.Target.Funcs, foreign)
	n := len(a.Noder.Typecheck.Target.Funcs)
	defer func() {
		if recover() == nil {
			t.Error("foreign function accepted")
		}
		if len(a.Noder.Typecheck.Target.Funcs) != n {
			t.Error("mutated before validation")
		}
	}()
	s.GenABIWrappers()
}

func TestNativeABIRequiresExplicitInput(t *testing.T) {
	_, _, s := abiContext(t)
	defer func() {
		if x := recover(); x == nil || !strings.Contains(fmt.Sprint(x), "input reader is not configured") {
			t.Errorf("unexpected implicit input behavior: %v", x)
		}
	}()
	s.ReadSymABIs("input.symabis")
}
func TestNativeABIRejectsCgoWrapper(t *testing.T) {
	c, b, s := abiContext(t)
	c.Noder.Typecheck.Target.CgoPragmas = [][]string{{"cgo_export_static", "example/p.Export"}}
	readABIs(b, s, "ref example/p.Export ABI0")
	defer func() {
		if x := recover(); x == nil || !strings.Contains(fmt.Sprint(x), "cannot have ABI wrappers") {
			t.Errorf("cgo wrapper was not rejected: %v", x)
		}
	}()
	s.GenABIWrappers()
}
func TestNativeABIRejectsForeignSymbol(t *testing.T) {
	_, _, s := abiContext(t)
	other, _, _ := abiContext(t)
	defer func() {
		if recover() == nil {
			t.Error("foreign definition symbol accepted")
		}
	}()
	s.HasDef(other.Noder.Types.LocalPkg.Lookup("Asm"))
}

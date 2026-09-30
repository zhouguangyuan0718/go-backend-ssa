package amd64

import (
	"bytes"
	"fmt"
	"go-backend-ssa/internal/cmd/obj"
	"go-backend-ssa/internal/cmd/objabi"
	"go-backend-ssa/internal/dwarfgen"
	"go-backend-ssa/internal/escape"
	"go-backend-ssa/internal/gc"
	"go-backend-ssa/internal/ssa"
	"go-backend-ssa/internal/ssagen"
	"go-backend-ssa/internal/syntax"
	"go-backend-ssa/internal/types2"
	"go-backend-ssa/internal/walk"
	"strings"
	"testing"
)

func queueCompiler(t *testing.T, workers int, input string) (*gc.Context, *ssagen.SymABIs) {
	return loadQueueCompiler(t, workers, input, false)
}
func loadQueueCompiler(t *testing.T, workers int, input string, nativeFrontend bool) (*gc.Context, *ssagen.SymABIs) {
	t.Helper()
	backend := testBackendWorkers(true, workers)
	dwarfgen.NewContext(backend, backend.SSA.ReflectData)
	c := globalFinalizer(t, backend)
	n := c.Package.Noder
	n.Debug.SyncFrames = -1
	n.Flag.Lang = "go1.27"
	n.Flag.Complete = false
	file, err := syntax.Parse(syntax.NewFileBase("queue.go"), strings.NewReader(input), nil, nil, syntax.CheckBranches)
	if err != nil {
		t.Fatal(err)
	}
	info := &types2.Info{StoreTypesInSyntax: true, Defs: map[*syntax.Name]types2.Object{}, Uses: map[*syntax.Name]types2.Object{}, Selections: map[*syntax.SelectorExpr]*types2.Selection{}, Implicits: map[syntax.Node]types2.Object{}, Scopes: map[syntax.Node]*types2.Scope{}, Instances: map[*syntax.Name]types2.Instance{}, FileVersions: map[*syntax.PosBase]string{}}
	config := types2.Config{Context: n.Checker, GoVersion: "go1.27", Sizes: n.Checker.SizesFor("gc", "amd64")}
	pkg, err := config.Check("example/p", []*syntax.File{file}, info)
	if err != nil {
		t.Fatal(err)
	}
	closures, err := n.RewriteRangeFuncs(pkg, info, []*syntax.File{file})
	if err != nil {
		t.Fatal(err)
	}
	data, err := n.WritePackage(pkg, info, []*syntax.File{file}, closures)
	if err != nil {
		t.Fatal(err)
	}
	n.ReadLocalPackage(data)

	if !nativeFrontend {
		n.MakeWrappers(n.Typecheck.Target)
		c.Package.MakeTask()
	}
	backend.Inline = n.Inline
	backend.ReadFile = func(string) ([]byte, error) { return []byte("def example/p.Assembly ABI0\n"), nil }
	symABIs := backend.NewSymABIs()
	symABIs.ReadSymABIs("queue.symabis")
	if !nativeFrontend {
		symABIs.GenABIWrappers()
	}
	esc := escape.NewContext(n.Typecheck, n.Inline.Log)
	if !nativeFrontend {
		esc.Funcs(n.Typecheck.Target.Funcs)
	}
	c.BindBackend(backend, walk.NewContext(n, esc))
	return c, symABIs
}
func TestNativeCompileQueue(t *testing.T) {
	for i := range 8 {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			t.Parallel()
			c, syms := queueCompiler(t, 4, fmt.Sprintf(`package p
func _(x int) int {return x}
func Assembly(p *int) int
func Value(x int) int {return x+%d}
func Closure(x int) func() func() int {return func()func()int{return func()int{return x}}}
func Call(p *int) int {return Assembly(p)}
`, i+1))
			c.RaceEnabled = i%2 == 0
			c.Writer.Types.Experiment.FieldTrack = true
			trackedA := c.Writer.Link.Lookup("go:track.A")
			trackedZ := c.Writer.Link.Lookup("go:track.Z")
			for _, fn := range c.Package.Noder.Typecheck.Target.Funcs {
				if fn.Sym().Name == "Value" {
					fn.FieldTrack = map[*obj.LSym]struct{}{trackedZ: {}, trackedA: {}}
				}
			}
			for _, fn := range c.Package.Noder.Typecheck.Target.Funcs {
				c.EnqueueFunc(fn, syms)
			}
			if c.QueuedFunctions() != 3 {
				t.Fatal("queue does not distinguish blank and bodyless functions", c.QueuedFunctions())
			}
			c.CompileQueued(nil)
			c.Backend.CheckLargeStacks()
			if c.QueuedFunctions() != 0 || c.Writer.Link.InParallel || c.Writer.Types.CalcSizeDisabled || c.Package.Noder.Typecheck.IR.CurFunc != nil {
				t.Fatal("queue lifecycle flags leaked")
			}
			var retained [][]byte
			var before [][]byte
			compiled := 0
			for _, fn := range c.Package.Noder.Typecheck.Target.Funcs {
				if fn.Sym().Name == "_" {
					if fn.LSym != nil {
						t.Fatal("blank function emitted")
					}
					continue
				}
				if fn.Sym().Name == "Assembly" {
					stackmap, arginfo := false, false
					for _, data := range c.Writer.Link.Data {
						if data.Name == fn.LSym.Name+".args_stackmap" {
							stackmap = len(data.P) > 8 && data.P[8]&1 == 1
						}
						if data.Name == fn.LSym.Name+".arginfo0" {
							arginfo = len(data.P) > 0
						}
					}
					if !stackmap || !arginfo {
						t.Fatal("bodyless ABI0 stack map missing")
					}
					continue
				}
				if len(fn.LSym.P) == 0 {
					t.Fatal("queue missed function", fn)
				}
				if fn.Sym().Name == "Value" {
					var tracked []string
					for _, rel := range fn.LSym.R {
						if rel.Type == objabi.R_USEFIELD {
							tracked = append(tracked, rel.Sym.Name)
						}
					}
					if strings.Join(tracked, ",") != "go:track.A,go:track.Z" {
						t.Fatal("field relocations missing or unordered", tracked)
					}
				}
				retained = append(retained, fn.LSym.P)
				before = append(before, bytes.Clone(fn.LSym.P))
				compiled++
			}
			if compiled < 5 {
				t.Fatal("nested closures not compiled", compiled)
			}

			other, otherABIs := queueCompiler(t, 1, "package p;func Other()int{return 99}")
			for _, fn := range other.Package.Noder.Typecheck.Target.Funcs {
				other.EnqueueFunc(fn, otherABIs)
			}
			other.CompileQueued(nil)
			for j, data := range retained {
				if !bytes.Equal(data, before[j]) {
					t.Fatal("other compilation changed retained code")
				}
			}
		})
	}
}
func TestCompileQueuePreflight(t *testing.T) {
	a, abis := queueCompiler(t, 1, "package p;func A()int{return 1}")
	b, babis := queueCompiler(t, 1, "package p;func B()int{return 2}")
	fn := a.Package.Noder.Typecheck.Target.Funcs[0]
	mustPanic := func(f func()) {
		t.Helper()
		defer func() {
			if recover() == nil {
				t.Error("invalid queue input accepted")
			}
		}()
		f()
	}
	mustPanic(func() { a.EnqueueFunc(b.Package.Noder.Typecheck.Target.Funcs[0], abis) })
	mustPanic(func() { a.EnqueueFunc(fn, babis) })
	if fn.LSym != nil || a.QueuedFunctions() != 0 {
		t.Fatal("preflight mutated local graph")
	}
	a.EnqueueFunc(fn, abis)
	mustPanic(func() { a.AddGCLocals() })
	a.Package.Noder.Flag.LowerC = 2
	mustPanic(func() { a.CompileQueued(nil) })
	a.Package.Noder.Flag.LowerC = 1
	mustPanic(func() { a.EnqueueFunc(fn, abis) })
	a.CompileQueued(nil)
	if len(fn.LSym.P) == 0 {
		t.Fatal("valid queue failed after rejected input")
	}
}

func TestCompileQueueLargeStack(t *testing.T) {
	c, abis := queueCompiler(t, 2, "package p;func TooLarge(x [1<<30]byte){};func Fine()int{return 7}")
	for _, fn := range c.Package.Noder.Typecheck.Target.Funcs {
		c.EnqueueFunc(fn, abis)
	}
	c.CompileQueued(nil)
	if c.Package.Noder.Typecheck.IR.Diagnostics.HasErrors() {
		t.Fatal("large frame diagnosed before ordered final check")
	}
	c.Backend.CheckLargeStacks()
	found := false
	for _, d := range c.Package.Noder.Typecheck.IR.Diagnostics.Diagnostics() {
		found = found || strings.Contains(d.Message, "stack frame too large (>1GB)")
	}
	if !found {
		t.Fatal("large stack diagnostic missing")
	}
	for _, fn := range c.Package.Noder.Typecheck.Target.Funcs {
		if fn.Sym().Name == "TooLarge" && len(fn.LSym.P) != 0 {
			t.Fatal("oversized frame reached assembler")
		}
		if fn.Sym().Name == "Fine" && len(fn.LSym.P) == 0 {
			t.Fatal("other worker did not complete")
		}
	}
}

func TestCompileQueueConcurrentDiagnostics(t *testing.T) {
	var source strings.Builder
	source.WriteString("package p\n")
	for i := range 32 {
		fmt.Fprintf(&source, "func F%d(){defer func(){}()}\n", i)
	}
	c, abis := queueCompiler(t, 4, source.String())
	c.Package.Noder.Debug.Defer = 1
	for _, fn := range c.Package.Noder.Typecheck.Target.Funcs {
		c.EnqueueFunc(fn, abis)
	}
	c.CompileQueued(nil)
	count := 0
	for _, d := range c.Package.Noder.Typecheck.IR.Diagnostics.Diagnostics() {
		if strings.Contains(d.Message, "defer") {
			count++
		}
	}
	if count != 32 {
		t.Fatal("worker diagnostics lost", count)
	}
}

func TestCompileQueueWorkerFailure(t *testing.T) {
	c, abis := queueCompiler(t, 4, "package p;func A()int{return 1};func B()int{return 2}")
	marker := new(int)
	c.Backend.Arch.SSAGenValue = func(*ssagen.State, *ssa.Value) { panic(marker) }
	for _, fn := range c.Package.Noder.Typecheck.Target.Funcs {
		c.EnqueueFunc(fn, abis)
	}
	func() {
		defer func() {
			if got := recover(); got != marker {
				t.Errorf("worker panic identity not returned to caller: %v", got)
			}
		}()
		c.CompileQueued(nil)
		t.Error("worker failure accepted")
	}()
	if c.Writer.Link.InParallel || c.Writer.Types.CalcSizeDisabled {
		t.Fatal("failed worker left parallel phase active")
	}
	for _, op := range []func(){func() { c.CompileQueued(nil) }, func() { c.AddGCLocals() }} {
		func() {
			defer func() {
				if recover() == nil {
					t.Error("failed graph reused")
				}
			}()
			op()
		}()
	}
	other, otherABIs := queueCompiler(t, 1, "package p;func C()int{return 3}")
	fn := other.Package.Noder.Typecheck.Target.Funcs[0]
	other.EnqueueFunc(fn, otherABIs)
	other.CompileQueued(nil)
	if len(fn.LSym.P) == 0 {
		t.Fatal("new instance failed after worker panic")
	}
}

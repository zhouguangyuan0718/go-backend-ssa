package pkginit

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"go-backend-ssa/internal/cmd/obj"
	"go-backend-ssa/internal/cmd/objabi"
	"go-backend-ssa/internal/cmd/sys"
	"go-backend-ssa/internal/ir"
	"go-backend-ssa/internal/loopvar"
	"go-backend-ssa/internal/noder"
	"go-backend-ssa/internal/objw"
	"go-backend-ssa/internal/reflectdata"
	"go-backend-ssa/internal/rttype"
	"go-backend-ssa/internal/staticdata"
	"go-backend-ssa/internal/staticinit"
	"go-backend-ssa/internal/syntax"
	"go-backend-ssa/internal/typecheck"
	"go-backend-ssa/internal/types"
	"go-backend-ssa/internal/types2"
	"strings"
	"testing"
)

func sourceContext(t *testing.T, path, source string, asan bool, configure ...func(*noder.Context)) *Context {
	t.Helper()
	link := &obj.Link{Pathname: "/src", Pkgpath: path, Arch: &obj.LinkArch{Arch: &sys.Arch{Name: "amd64", PtrSize: 8, RegSize: 8, ByteOrder: binary.LittleEndian}}}
	u := types.NewUniverseWithPositions(8, 8, 1<<50, &link.PosTable)
	u.LocalPkg = u.NewPkg(path, "p")
	graph := ir.NewContext(u, link)
	tc := typecheck.NewContext(graph)
	tc.InitUniverse()
	tc.InitRuntime()
	graph.Base.Flag.Lang = "go1.27"
	graph.Base.Flag.Complete = true
	graph.Base.Flag.ASan = asan
	graph.Debug.SyncFrames = -1
	graph.Debug.RangeFuncCheck = 1
	nd, err := noder.NewContext(types2.NewContext(), tc)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range configure {
		f(nd)
	}
	rt := rttype.NewContext(objw.NewContext(u, link, "linux"))
	rt.Init()
	nd.ReflectData = reflectdata.NewContext(tc, rt, staticdata.NewContext(graph, rt.Writer))
	nd.StaticInit = staticinit.NewContext(tc, nd.ReflectData)
	f, err := syntax.Parse(syntax.NewFileBase("input.go"), strings.NewReader(source), nil, nil, syntax.CheckBranches)
	if err != nil {
		t.Fatal(err)
	}
	info := &types2.Info{StoreTypesInSyntax: true, Defs: map[*syntax.Name]types2.Object{}, Uses: map[*syntax.Name]types2.Object{}, Selections: map[*syntax.SelectorExpr]*types2.Selection{}, Implicits: map[syntax.Node]types2.Object{}, Scopes: map[syntax.Node]*types2.Scope{}, Instances: map[*syntax.Name]types2.Instance{}, FileVersions: map[*syntax.PosBase]string{}}
	conf := types2.Config{Context: nd.Checker, GoVersion: "go1.27", Sizes: nd.Checker.SizesFor("gc", "amd64")}
	pkg, err := conf.Check(path, []*syntax.File{f}, info)
	if err != nil {
		t.Fatal(err)
	}
	rewritten, err := nd.RewriteRangeFuncs(pkg, info, []*syntax.File{f})
	if err != nil {
		t.Fatal(err)
	}
	stub, err := nd.WritePackage(pkg, info, []*syntax.File{f}, rewritten)
	if err != nil {
		t.Fatal(err)
	}
	target := nd.ReadLocalPackage(stub)
	nd.Interleaved.DevirtualizeAndInlinePackage(target, nil)
	nd.MakeWrappers(target)
	capture := loopvar.NewContext(tc, nd.Inline.Log)
	for _, fn := range target.Funcs {
		capture.ForCapture(fn)
	}
	return NewContext(nd)
}

const initSource = `package p
var Static=42
var Array=[3]int{1,2,3}
var Side []int
//go:noinline
func effect()int{Side=append(Side,7);return len(Side)}
var Dynamic=effect()
func init(){Dynamic+=2}
func init(){}
`

func initTask(c *Context) *obj.LSym {
	n, _ := c.Noder.Types.LocalPkg.Lookup(".inittask").Def.(*ir.Name)
	if n == nil {
		return nil
	}
	return n.Linksym()
}
func TestNativeMakeTask(t *testing.T) {
	c := sourceContext(t, "example/p", initSource, false)
	// Native imported init dependency metadata, as decoded by the reader.
	u := c.Noder.Types
	dep := u.NewPkg("example/dep", "dep")
	name := c.Noder.Typecheck.IR.NewNameAt(u.Pos, dep.Lookup(".inittask"), u.Types[types.TUINT8])
	name.Class = ir.PEXTERN
	name.Sym().Def = name
	c.Noder.Typecheck.Target.Imports = append(c.Noder.Typecheck.Target.Imports, dep)
	c.MakeTask()
	task := initTask(c)
	if task == nil || task.Size != 24 || len(task.R) != 3 || binary.LittleEndian.Uint32(task.P[4:8]) != 2 {
		t.Fatal("task layout", task)
	}
	if task.R[2].Type != objabi.R_INITORDER || task.R[2].Sym != name.Linksym() {
		t.Fatal("dependency order", task.R)
	}
	if task.R[0].Sym.Name != "example/p.init" || task.R[1].Sym.Name != "example/p.init.0" {
		t.Fatal("init function order", task.R)
	}
	n := u.LocalPkg.Lookup("Static").Def.(*ir.Name)
	if binary.LittleEndian.Uint64(n.Linksym().P) != 42 {
		t.Fatal("static data lost")
	}
	if c.Noder.Typecheck.IR.CurFunc != nil {
		t.Fatal("current function leaked")
	}
}
func TestNativeMakeTaskEmpty(t *testing.T) {
	for _, path := range []string{"example/p", "main", "runtime"} {
		t.Run(path, func(t *testing.T) {
			c := sourceContext(t, path, "package p;func init(){}", false)
			c.MakeTask()
			task := initTask(c)
			if path == "example/p" {
				if task != nil {
					t.Fatal("empty package task")
				}
			} else if task == nil || task.Size != 8 || len(task.R) != 0 {
				t.Fatal("required empty task", task)
			}
		})
	}
	c := sourceContext(t, "example/p", "package p;var A=42", false)
	c.MakeTask()
	if initTask(c) != nil {
		t.Fatal("fully static package gained task")
	}
}
func TestNativeMakeTaskASan(t *testing.T) {
	c := sourceContext(t, "example/p", initSource+"\nvar cgoSkip int;var Linked int", true)
	c.Noder.Types.LocalPkg.Lookup("Linked").Linkname = "external.Linked"
	c.MakeTask()
	if len(c.InstrumentGlobalsMap) != 4 || len(c.InstrumentGlobalsSlice) != 4 {
		t.Fatal("instrument selection", c.InstrumentGlobalsMap)
	}
	target := c.Noder.Typecheck.Target
	fn := target.Inits[len(target.Inits)-1]
	if fn.Sym().Name != "init.2" {
		t.Fatal("ASan init naming", fn)
	}
	last, ok := fn.Body[len(fn.Body)-1].(*ir.CallExpr)
	if !ok || last.Fun.Sym().Name != "asanregisterglobals" || c.Noder.Typecheck.IR.Int64Val(last.Args[1]) != 4 {
		t.Fatal("ASan registration", last)
	}
	globals := c.Noder.Types.LocalPkg.Lookup(".asanglobals").Def.(*ir.Name)
	if globals.Type().NumElem() != 4 || globals.Type().Elem().Size() != 64 {
		t.Fatal("ASan descriptor layout", globals.Type())
	}
	if initTask(c) == nil || binary.LittleEndian.Uint32(initTask(c).P[4:8]) != 3 {
		t.Fatal("ASan init missing from task")
	}
	if c.Noder.Typecheck.IR.CurFunc != nil {
		t.Fatal("ASan current function leaked")
	}
}
func TestNativeMakeTaskIsolation(t *testing.T) {
	old := sourceContext(t, "example/p", initSource, true)
	old.MakeTask()
	task := initTask(old)
	data := append([]byte(nil), task.P...)
	body := fmt.Sprint(old.Noder.Typecheck.Target.Inits)
	for i := 0; i < 8; i++ {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			t.Parallel()
			c := sourceContext(t, "example/p", initSource, true)
			c.MakeTask()
			if initTask(c) == task || !bytes.Equal(task.P, data) || fmt.Sprint(old.Noder.Typecheck.Target.Inits) != body || len(old.InstrumentGlobalsMap) != 4 {
				t.Fatal("retained state changed")
			}
		})
	}
}
func TestNativeMakeTaskRejectsForeignState(t *testing.T) {
	a, b := sourceContext(t, "example/p", initSource, false), sourceContext(t, "example/q", initSource, false)
	a.Noder.Typecheck.Target.Inits = append(a.Noder.Typecheck.Target.Inits, b.Noder.Typecheck.Target.Inits[0])
	defer func() {
		if recover() == nil {
			t.Error("foreign init accepted")
		}
		if initTask(a) != nil {
			t.Error("mutated task before rejection")
		}
	}()
	a.MakeTask()
}
func TestScheduleLiteralBinding(t *testing.T) {
	c := sourceContext(t, "example/p", "package p", false)
	plans := make(map[ir.Node]*staticinit.Plan)
	temps := make(map[ir.Node]*ir.Name)
	s := c.Noder.StaticInit.BindSchedule(staticinit.Schedule{Plans: plans, Temps: temps})
	n := c.Noder.Typecheck.IR.NewInt(c.Noder.Types.Pos, 1)
	plans[n] = &staticinit.Plan{}
	if s.Plans[n] != plans[n] {
		t.Fatal("map initializer replaced")
	}
	defer func() {
		if recover() == nil {
			t.Error("bound schedule accepted")
		}
	}()
	c.Noder.StaticInit.BindSchedule(s)
}

func TestInitializationContextDoesNotInternImports(t *testing.T) {
	c := sourceContext(t, "example/p", "package p", false)
	dep := c.Noder.Types.NewPkg("example/dep", "dep")
	c.Noder.Typecheck.Target.Imports = append(c.Noder.Typecheck.Target.Imports, dep)
	before := len(dep.Syms)
	NewContext(c.Noder)
	if len(dep.Syms) != before {
		t.Fatal("context validation changed import symbols")
	}
}
func TestInitializationRejectsForeignImportedTask(t *testing.T) {
	c := sourceContext(t, "example/p", "package p", false)
	other := sourceContext(t, "example/q", initSource, false)
	dep := c.Noder.Types.NewPkg("example/dep", "dep")
	dep.Lookup(".inittask").Def = other.Noder.Typecheck.Target.Inits[0].Nname
	c.Noder.Typecheck.Target.Imports = append(c.Noder.Typecheck.Target.Imports, dep)
	before := len(c.Noder.Ctxt.Data)
	defer func() {
		if recover() == nil {
			t.Error("foreign imported init task accepted")
		}
		if len(c.Noder.Ctxt.Data) != before {
			t.Error("data mutated before ownership check")
		}
	}()
	c.MakeTask()
}

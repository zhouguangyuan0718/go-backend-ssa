package ssagen

import (
	"bytes"
	"fmt"
	"go-backend-ssa/internal/abi"
	"go-backend-ssa/internal/cmd/obj"
	"go-backend-ssa/internal/cmd/src"
	"go-backend-ssa/internal/cmd/sys"
	"go-backend-ssa/internal/ir"
	"go-backend-ssa/internal/logopt"
	"go-backend-ssa/internal/objw"
	"go-backend-ssa/internal/reflectdata"
	"go-backend-ssa/internal/rttype"
	"go-backend-ssa/internal/ssa"
	"go-backend-ssa/internal/staticdata"
	"go-backend-ssa/internal/typecheck"
	"go-backend-ssa/internal/types"
	"strings"
	"testing"
	"time"
)

func frontendContext(arch string, soft bool) *Context {
	var target *sys.Arch
	for _, a := range sys.NewContext().Archs {
		if a.Name == arch {
			target = a
		}
	}
	if target == nil {
		panic(arch)
	}
	link := &obj.Link{Arch: &obj.LinkArch{Arch: target}, Pkgpath: "example/p"}
	u := types.NewUniverseWithPositions(target.PtrSize, target.RegSize, 1<<50, &link.PosTable)
	u.LocalPkg = u.NewPkg("example/p", "p")
	g := ir.NewContext(u, link)
	tc := typecheck.NewContext(g)
	tc.InitUniverse()
	tc.InitRuntime()
	g.Base.Flag.LowerC = 3
	g.Base.Flag.WB = true
	rt := rttype.NewContext(objw.NewContext(u, link, "linux"))
	rt.Init()
	s := ssa.NewContext(abi.NewContext(g), reflectdata.NewContext(tc, rt, staticdata.NewContext(g, rt.Writer)), logopt.NewContext())
	s.GOAMD64 = 1
	s.GOARM.Version = 7
	s.GOARM.SoftFloat = soft
	s.GOPPC64 = 8
	s.GORISCV64 = 20
	s.FramePointerEnabled = arch == "amd64" || arch == "arm64"
	s.Now = time.Now
	c := NewContext(tc)
	c.SSA = s
	c.Arch.LinkArch, c.Arch.SoftFloat = link.Arch, soft
	return c
}

func frontendFunction(c *Context, name string) *ir.Func {
	g, u := c.Typecheck.IR, c.Typecheck.IR.Types
	pos := g.Base.Ctxt.PosTable.XPos(src.MakePos(src.NewFileBase("frontend.go", "/src/frontend.go"), 17, 3))
	fn := g.NewFunc(pos, pos, u.LocalPkg.Lookup(name), u.NewSignature(nil, nil, nil))
	fn.DeclareParams(true)
	return fn
}

func TestNativeFrontendInitialization(t *testing.T) {
	for _, arch := range []string{"386", "amd64", "arm", "arm64", "loong64", "mips", "mipsle", "mips64", "mips64le", "ppc64", "ppc64le", "riscv64", "s390x", "wasm"} {
		t.Run(arch, func(t *testing.T) {
			for _, soft := range []bool{false, true} {
				c := frontendContext(arch, soft)
				g, u := c.Typecheck.IR, c.Typecheck.IR.Types
				g.Base.Flag.Race = true
				c.InitConfig()
				cfg := c.SSAConfig()
				if cfg == nil || cfg.Types.Int != u.Types[types.TINT] || cfg.SoftFloat != soft || !cfg.Race || u.NewPtrCacheEnabled || len(c.ssaCaches) != 3 {
					t.Fatal("configuration policy")
				}
				if (len(c.softFloatOps) != 0) != soft {
					t.Fatal("softfloat table")
				}
				if g.Syms.WriteBarrier != c.Typecheck.LookupRuntimeVar("writeBarrier") || g.Syms.GCWriteBarrier[7] != c.Typecheck.LookupRuntimeFunc("gcWriteBarrier8") || g.Syms.ZeroVal == nil {
					t.Fatal("runtime symbol identities")
				}
				for _, sym := range c.BoundsCheckFunc {
					if (sym != nil) != (arch == "wasm") {
						t.Fatal("bounds policy")
					}
				}
				deferType := c.deferstruct()
				if deferType != c.deferstruct() || deferType.NumFields() != 7 || deferType.Field(4).Sym.Name != "fn" || deferType.Size() != 6*int64(u.PointerSize()) {
					t.Fatal("defer layout/cache")
				}
				for _, f := range deferType.Fields() {
					if f.Sym.Universe() != u {
						t.Fatal("anonymous field ownership")
					}
				}
				fn := frontendFunction(c, "policy")
				for _, regabi := range []bool{false, true} {
					u.Experiment.RegabiArgs = regabi
					want := cfg.ABI0
					if regabi {
						want = cfg.ABI1
					}
					if c.abiForFunc(nil, cfg.ABI0, cfg.ABI1) != want {
						t.Fatal("default ABI")
					}
					fn.ABI = obj.ABI0
					if c.abiForFunc(fn, cfg.ABI0, cfg.ABI1) != cfg.ABI0 || c.AbiForBodylessFuncStackMap(fn) != cfg.ABI0 {
						t.Fatal("stack ABI")
					}
					fn.ABI = obj.ABIInternal
					if c.abiForFunc(fn, cfg.ABI0, cfg.ABI1) != want {
						t.Fatal("internal ABI")
					}
				}
			}
		})
	}
}

func TestNativeFrontendServices(t *testing.T) {
	c := frontendContext("amd64", false)
	c.InitConfig()
	g, u := c.Typecheck.IR, c.Typecheck.IR.Types
	fn := frontendFunction(c, "services")
	fe := c.NewFrontend(fn, true)
	var trace bytes.Buffer
	g.TraceWriter = &trace
	fe.Logf("frontend %d", 17)
	if !fe.Log() || trace.String() != "frontend 17" || fe.Func() != fn {
		t.Fatal("frontend identity/logging")
	}
	c.NewFrontend(fn, false).Logf("hidden")
	if trace.String() != "frontend 17" {
		t.Fatal("disabled logging")
	}
	g.Debug.Nil = 1
	if !fe.Debug_checknil() || !fe.UseWriteBarrier() {
		t.Fatal("frontend flags")
	}
	g.Debug.Nil = 0
	g.Base.Flag.WB = false
	if fe.Debug_checknil() || fe.UseWriteBarrier() {
		t.Fatal("frontend flags retained")
	}
	for _, name := range []string{"goschedguarded", "writeBarrier", "wbZero", "wbMove", "cgoCheckMemmove", "cgoCheckPtrWrite"} {
		if sym := fe.Syslook(name); sym == nil || sym.Name != "runtime."+name {
			t.Fatalf("runtime lookup %s: %v", name, sym)
		}
	}
	data := fe.StringData("a\x00β")
	if data != fe.StringData("a\x00β") || string(data.P) != "a\x00β" || data != c.NewFrontend(fn, false).StringData("a\x00β") {
		t.Fatal("native string storage/cache")
	}
	parent := ssa.LocalSlot{N: fn.NewLocal(fn.Pos(), u.LocalPkg.Lookup("slot"), u.Types[types.TSTRING]), Type: u.Types[types.TSTRING], Off: 16}
	for _, addressed := range []bool{false, true} {
		parent.N.SetAddrtaken(addressed)
		before := len(fn.Dcl)
		slot := fe.SplitSlot(&parent, ".len", 8, u.Types[types.TINT])
		if addressed {
			if slot.N != parent.N || slot.Off != 24 || len(fn.Dcl) != before || slot.SplitOf != nil {
				t.Fatal("addressed slot")
			}
		} else {
			second := fe.SplitSlot(&parent, ".len", 8, u.Types[types.TINT])
			if slot.N == parent.N || slot.N.Curfn != fn || !slot.N.Used() || slot.N.Esc() != ir.EscNever || slot.SplitOf != &parent || slot.SplitOffset != 8 || slot.Off != 0 || slot.N.Sym() == u.LocalPkg.Lookup("slot.len") || slot.N.Sym() == second.N.Sym() || slot.N.Sym().Universe() != u {
				t.Fatal("split slot identity")
			}
		}
	}
	parent.N.SetAddrtaken(false)
	parent.N.Class = ir.PPARAM
	if slot := fe.SplitSlot(&parent, ".len", 8, u.Types[types.TINT]); slot.N != parent.N || slot.Off != 24 {
		t.Fatal("parameter slot")
	}
	fe.Warnl(fn.Pos(), "warning %d", 23)
	d := g.Diagnostics.Diagnostics()
	if len(d) != 1 || d[0].Line != 17 || d[0].Message != "warning 23" {
		t.Fatal(d)
	}
	frontendPanic(t, "unknown Syslook", func() { fe.Syslook("mallocgc") })
	frontendPanic(t, "'services': failure 42", func() { fe.Fatalf(fn.Pos(), "failure %d", 42) })
	if u.Pos != fn.Pos() {
		t.Fatal("fatal position")
	}
}

func frontendPanic(t *testing.T, contains string, f func()) {
	t.Helper()
	defer func() {
		if e := recover(); e == nil || !strings.Contains(fmt.Sprint(e), contains) {
			t.Errorf("panic = %v, want %q", e, contains)
		}
	}()
	f()
}

func TestNativeFrontendParallelCompile(t *testing.T) {
	for i := 0; i < 8; i++ {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			t.Parallel()
			c := frontendContext([]string{"amd64", "arm64", "riscv64", "wasm"}[i%4], false)
			c.InitConfig()
			fn := frontendFunction(c, fmt.Sprintf("parallel%d", i))
			fe := c.NewFrontend(fn, false)
			cache := new(ssa.Cache)
			cache.Reset()
			f := c.SSAConfig().NewFunc(fe, cache)
			f.Name, f.Type = fn.Sym().Name, fn.Type()
			f.ABI0, f.ABI1 = c.SSAConfig().ABI0, c.SSAConfig().ABI1
			f.ABISelf, f.ABIDefault = f.ABI1, f.ABI1
			f.OwnAux = ssa.OwnAuxCall(fn.Linksym(), f.ABISelf.ABIAnalyze(fn.Type(), true))
			f.Entry = f.NewBlock(ssa.BlockRet)
			f.Entry.Pos = fn.Pos()
			mem := f.Entry.NewValue0(fn.Pos(), ssa.OpInitMem, c.SSA.Types.TypeMem)
			// Force the native frontend through string materialization, aggregate
			// splitting, write-barrier lookup and a real call in the full pipeline.
			u := c.SSA.Types
			local := fn.NewLocal(fn.Pos(), u.LocalPkg.Lookup("str"), f.Config.Types.String)
			f.SplitString(&ssa.LocalSlot{N: local, Type: local.Type()})
			sb := f.Entry.NewValue0(fn.Pos(), ssa.OpSB, f.Config.Types.Uintptr)
			pointerCell := u.NewPtr(f.Config.Types.BytePtr)
			source := f.Entry.NewValue1A(fn.Pos(), ssa.OpAddr, pointerCell, c.Typecheck.IR.Base.Ctxt.Lookup("example/p.source"), sb)
			dest := f.Entry.NewValue1A(fn.Pos(), ssa.OpAddr, pointerCell, c.Typecheck.IR.Base.Ctxt.Lookup("example/p.dest"), sb)
			ptr := f.Entry.NewValue2(fn.Pos(), ssa.OpLoad, f.Config.Types.BytePtr, source, mem)
			mem = f.Entry.NewValue3A(fn.Pos(), ssa.OpStore, u.TypeMem, f.Config.Types.BytePtr, dest, ptr, mem)
			str := f.Entry.NewValue0A(fn.Pos(), ssa.OpConstString, f.Config.Types.String, ssa.StringToAux("native frontend string"))
			callABI := f.ABIDefault.ABIAnalyzeTypes([]*types.Type{f.Config.Types.String}, nil)
			aux := ssa.StaticAuxCall(c.Typecheck.IR.Base.Ctxt.Lookup("example/p.consume"), callABI)
			call := f.Entry.NewValue0A(fn.Pos(), ssa.OpStaticLECall, aux.LateExpansionResultType(), aux)
			call.AddArgs(str, mem)
			mem = f.Entry.NewValue1I(fn.Pos(), ssa.OpSelectN, u.TypeMem, 0, call)
			ret := f.Entry.NewValue1(fn.Pos(), ssa.OpMakeResult, c.SSA.Types.TypeResultMem, mem)
			f.Entry.SetControl(ret)
			if err := c.SSA.PhaseOption("check", "on", 1, ""); err != "" {
				t.Fatal(err)
			}
			ssa.Compile(f)
			if f.RegAlloc == nil {
				t.Fatal("not compiled")
			}
			if !strings.Contains(f.String(), "writeBarrier") || fe.(*ssafn).strings["native frontend string"] == nil || len(fn.Dcl) < 3 {
				t.Fatal("native frontend callbacks did not run")
			}
			other := frontendContext("amd64", false)
			other.InitConfig()
			frontendPanic(t, "declared function", func() { other.NewFrontend(fn, false) })
			parent := ssa.LocalSlot{N: fn.NewLocal(fn.Pos(), c.SSA.Types.LocalPkg.Lookup("local"), c.SSA.Types.Types[types.TINT]), Type: c.SSA.Types.Types[types.TINT]}
			before := len(fn.Dcl)
			frontendPanic(t, "another compilation", func() { fe.SplitSlot(&parent, ".foreign", 0, other.SSA.Types.Types[types.TINT]) })
			if len(fn.Dcl) != before || c.deferType == other.deferType || fe.StringData("same") == other.NewFrontend(frontendFunction(other, "other"), false).StringData("same") {
				t.Fatal("compilation isolation")
			}
		})
	}
}

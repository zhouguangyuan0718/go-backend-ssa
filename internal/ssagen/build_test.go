package ssagen

import (
	"fmt"
	"strings"
	"testing"

	"go-backend-ssa/internal/ir"
	"go-backend-ssa/internal/ssa"
	"go-backend-ssa/internal/types"
)

func buildContext(arch string) *Context {
	c := frameContext(arch, true)
	c.GO386 = "sse2"
	c.GOMIPS = "hardfloat"
	c.GOMIPS64 = "hardfloat"
	c.GOARM64.Version = "v8.0"
	c.InitTables()
	if err := c.SSA.PhaseOption("check", "on", 1, ""); err != "" {
		panic(err)
	}
	return c
}

func TestNativeBuildSSAAndIntrinsicContexts(t *testing.T) {
	for _, arch := range []string{"386", "amd64", "arm", "arm64", "loong64", "mips", "mipsle", "mips64", "mips64le", "ppc64", "ppc64le", "riscv64", "s390x", "wasm"} {
		t.Run(arch, func(t *testing.T) {
			c := buildContext(arch)
			g := c.Typecheck.IR
			fn := frontendFunction(c, "empty")
			g.InitLSym(fn, true)
			fn.Pragma |= ir.Nosplit
			fn.Body = []ir.Node{g.NewReturnStmt(fn.Pos(), nil)}
			f := c.BuildSSA(fn, 0, true)
			if f.Frontend().Func() != fn || !f.NoSplit || !f.IsPgoHot || f.RegAlloc == nil || fn.DebugInfo == nil {
				t.Fatal("native build pipeline did not finish")
			}
			fe := f.Frontend().(*ssafn)
			if fe.stksize != 0 || fe.stkptrsize != 0 || fe.stkalign != int64(g.Types.PointerSize()) {
				t.Error(fe.stksize, fe.stkptrsize, fe.stkalign)
			}
			for _, b := range f.Blocks {
				for _, v := range b.Values {
					if v.Op == ssa.OpFwdRef {
						t.Fatal("unresolved variable")
					}
				}
			}
			sym := g.Types.NewPkg("internal/runtime/sys", "sys").Lookup("GetCallerPC")
			if !c.IsIntrinsicSym(sym) || !g.IsIntrinsicSym(sym) {
				t.Fatal("native symbol intrinsic hook not installed")
			}
			call := g.NewCallExpr(fn.Pos(), ir.OCALLFUNC, g.NewNameAt(fn.Pos(), sym, g.Types.NewSignature(nil, nil, nil)), nil)
			if !c.IsIntrinsicCall(call) || !g.IsIntrinsicCall(call) {
				t.Fatal("native call intrinsic hook not installed")
			}
			if c.memVar == nil || c.memVar.Sym().Pkg != nil || !g.OwnsNode(c.memVar) || fmt.Sprint(c.memVar.Sym()) != "mem" {
				t.Fatal("native marker identity")
			}
		})
	}
	for i := range 8 {
		t.Run(fmt.Sprint("parallel-", i), func(t *testing.T) {
			t.Parallel()
			c := buildContext("amd64")
			other := buildContext("arm64")
			if c.memVar == other.memVar || c.memVar.Sym() == other.memVar.Sym() {
				t.Fatal("shared markers")
			}
			f := frontendFunction(c, "isolated")
			c.Typecheck.IR.InitLSym(f, true)
			f.Body = []ir.Node{c.Typecheck.IR.NewReturnStmt(f.Pos(), nil)}
			result := c.BuildSSA(f, 0, false)
			before := result.String()
			g := frontendFunction(other, "other")
			other.Typecheck.IR.InitLSym(g, true)
			g.Body = []ir.Node{other.Typecheck.IR.NewReturnStmt(g.Pos(), nil)}
			other.BuildSSA(g, 0, false)
			if result.String() != before {
				t.Fatal("retained compilation changed")
			}
		})
	}
}

func TestNativeBuildSSARejectsForeignGraphBeforeMutation(t *testing.T) {
	for _, where := range []string{"function", "body", "declaration", "worker", "intrinsics"} {
		t.Run(where, func(t *testing.T) {
			c, other := buildContext("amd64"), buildContext("amd64")
			fn := frontendFunction(c, "reject")
			foreign := frontendFunction(other, "foreign")
			c.Typecheck.IR.InitLSym(fn, true)
			other.Typecheck.IR.InitLSym(foreign, true)
			worker := 0
			switch where {
			case "function":
				fn = foreign
			case "body":
				fn.Body = []ir.Node{other.Typecheck.IR.NewReturnStmt(foreign.Pos(), nil)}
			case "declaration":
				fn.Dcl = append(fn.Dcl, other.Typecheck.TempAt(foreign.Pos(), foreign, other.Typecheck.IR.Types.Types[types.TINT]))
			case "worker":
				worker = 99
			case "intrinsics":
				c.intrinsics = nil
			}
			before := len(fn.Dcl)
			debug := fn.DebugInfo
			defer func() {
				got := recover()
				if got == nil || !strings.Contains(fmt.Sprint(got), map[bool]string{true: "compilation", false: "requires"}[where == "function" || where == "body" || where == "declaration"]) {
					t.Error("missing entry rejection", got)
				}
				if len(fn.Dcl) != before || fn.DebugInfo != debug {
					t.Error("native state changed before validation")
				}
			}()
			c.BuildSSA(fn, worker, false)
		})
	}
}

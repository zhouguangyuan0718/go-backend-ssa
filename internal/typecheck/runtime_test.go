package typecheck

import (
	"bytes"
	"fmt"
	"go-backend-ssa/internal/cmd/obj"
	"go-backend-ssa/internal/ir"
	"go-backend-ssa/internal/types"
	"strings"
	"sync"
	"testing"
)

func TestNativeRuntimeDeclarationsAndABIs(t *testing.T) {
	c := newTestContext(8)
	c.InitUniverse()
	c.InitRuntime()
	c.InitCoverage()
	u := c.IR.Types
	if c.IR.Pkgs.Runtime == u.NewPkg("runtime", "runtime") || c.IR.Pkgs.Runtime.Prefix != "runtime" || c.IR.Pkgs.Coverage.Prefix != "runtime/coverage" {
		t.Fatal("native pseudo-package identity")
	}
	for _, d := range c.runtimeDecls {
		n := c.LookupRuntime(d.name)
		if n.Sym().Def != n || n.Type() == nil {
			t.Fatalf("missing native declaration: %s", d.name)
		}
		switch d.tag {
		case funcTag:
			if n.Class != ir.PFUNC || n.Func == nil || n.Func.Nname != n || !n.Sym().Func() {
				t.Fatal(d.name)
			}
		case varTag:
			if n.Class != ir.PEXTERN || n.Sym().Func() {
				t.Fatal(d.name)
			}
		}
	}
	f := c.LookupRuntime("newobject")
	if f.Linksym() != c.LookupRuntimeFunc("newobject") || f.Linksym().ABI() != obj.ABIInternal || f.Linksym().Name != "runtime.newobject" {
		t.Fatal("native runtime function symbol")
	}
	if c.LookupRuntimeVar("newobject") == f.Linksym() || c.LookupRuntimeVar("newobject").ABI() != obj.ABI0 {
		t.Fatal("ABI namespaces collapsed")
	}
	v := c.LookupRuntime("writeBarrier")
	if v.Linksym() != c.LookupRuntimeVar("writeBarrier") || v.Linksym().Name != "runtime.writeBarrier" {
		t.Fatal("native runtime variable symbol")
	}
	coverage := c.LookupCoverage("initHook")
	if coverage.Linksym().Name != "runtime/coverage.initHook" || coverage.Class != ir.PFUNC {
		t.Fatal("native coverage declaration")
	}
	c.IR.Base.Timer.Stop("done")
	var out bytes.Buffer
	c.IR.Base.Timer.Write(&out, "")
	if !strings.Contains(out.String(), "fe:loadsys:done") {
		t.Fatalf("native timer not bound to instance: %s", out.String())
	}
}

func TestRuntimeSubstitutionDoesNotMutateDeclarations(t *testing.T) {
	c := newTestContext(8)
	c.InitUniverse()
	c.InitRuntime()
	u := c.IR.Types
	original := c.LookupRuntime("newobject")
	a := c.LookupRuntime("newobject", u.Types[types.TINT])
	b := c.LookupRuntime("newobject", u.Types[types.TSTRING])
	if a == original || a == b || a.Sym() != original.Sym() || a.Sym().Def != original || a.Func != original.Func || a.Type() == original.Type() || a.Type() == b.Type() {
		t.Fatal("native instantiated name identity")
	}
	if a.Type().Result(0).Type.Elem() != u.Types[types.TINT] || b.Type().Result(0).Type.Elem() != u.Types[types.TSTRING] || original.Type().Result(0).Type.Elem() != u.Types[types.TANY] {
		t.Fatal("placeholder substituted in shared declaration")
	}
	if a.Type().Param(0) == b.Type().Param(0) || a.Type().Result(0) == b.Type().Result(0) || a.Type().ResultsTuple() == b.Type().ResultsTuple() || a.Type().ResultsTuple().Field(0) != a.Type().Result(0) {
		t.Fatal("signature fields or result tuple alias across instantiations")
	}
	u.CalcSize(a.Type())
	u.CalcSize(b.Type())
	a.Type().Param(0).Offset = 123
	if b.Type().Param(0).Offset == 123 || original.Type().Param(0).Offset == 123 {
		t.Fatal("frame offset mutation escaped instantiated signature")
	}
	k, v := u.Types[types.TSTRING], u.Types[types.TINT]
	mapping := c.LookupRuntime("mapaccess1", k, v, k, v).Type()
	if mapping.Param(1).Type.Key() != k || mapping.Param(1).Type.Elem() != v || mapping.Param(2).Type.Elem() != k || mapping.Result(0).Type.Elem() != v {
		t.Fatal("native substitution traversal order")
	}
	other := newTestContext(8)
	other.InitUniverse()
	func() {
		defer func() {
			if recover() == nil {
				t.Error("foreign substitution type accepted")
			}
		}()
		c.LookupRuntime("newobject", other.IR.Types.Types[types.TINT])
	}()
	for _, args := range [][]*types.Type{{u.Types[types.TINT], u.Types[types.TSTRING]}, {u.Types[types.TINT]}} {
		func() {
			defer func() {
				if recover() == nil {
					t.Error("substitution arity mismatch accepted")
				}
			}()
			c.LookupRuntime("mapaccess1", args...)
		}()
	}
	func() {
		defer func() {
			if recover() == nil {
				t.Error("excess substitution arguments accepted")
			}
		}()
		c.LookupRuntime("newobject", u.Types[types.TINT], u.Types[types.TINT])
	}()
	if c.LookupRuntime("newobject") != original || original.Type().Result(0).Type.Elem() != u.Types[types.TANY] {
		t.Fatal("failed substitution damaged canonical declaration")
	}
}

func TestRuntimeInstancesConcurrentAndRetained(t *testing.T) {
	const count = 12
	contexts := make([]*Context, count)
	names := make([]*ir.Name, count)
	var wg sync.WaitGroup
	for i := range count {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c := newTestContext(4 + 4*(i%2))
			c.InitUniverse()
			c.InitRuntime()
			c.InitCoverage()
			names[i] = c.LookupRuntime("newobject", c.IR.Types.Types[types.TINT])
			contexts[i] = c
		}()
	}
	wg.Wait()
	for i, c := range contexts {
		n := names[i]
		typ := n.Type().Result(0).Type.Elem()
		if typ != c.IR.Types.Types[types.TINT] || typ.Size() != int64(4+4*(i%2)) {
			t.Fatal("retained runtime instance rebound")
		}
		if i > 0 && (n.Sym() == names[0].Sym() || n.Linksym() == names[0].Linksym() || c.IR.Pkgs.Runtime == contexts[0].IR.Pkgs.Runtime) {
			t.Fatal("runtime symbols shared across instances")
		}
	}
	if got := fmt.Sprint(names[0].Type()); !strings.Contains(got, "*int") {
		t.Fatal(got)
	}
}

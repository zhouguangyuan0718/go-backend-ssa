package types_test

import (
	"fmt"
	"go-backend-ssa/gcbridge"
	"go-backend-ssa/target"
	"go-backend-ssa/types"
	"slices"
	"sync"
	"testing"
)

func universe(t *testing.T, arch string) *types.Universe {
	t.Helper()
	l, err := target.NewLayout("linux", arch)
	if err != nil {
		t.Fatal(err)
	}
	u, err := types.NewUniverse(l)
	if err != nil {
		t.Fatal(err)
	}
	return u
}
func TestUniverseOwnershipAndNativeIdentity(t *testing.T) {
	a, b := universe(t, "amd64"), universe(t, "386")
	x, _ := a.Scalar(types.Int)
	y, _ := b.Scalar(types.Int)
	again, _ := a.Scalar(types.Int)
	if x == y || x != again || x.Universe() != a || x.Underlying() != x || a.PointerSize() != 8 || b.PointerSize() != 4 {
		t.Fatal("type or target state mixed")
	}
	x.SetHasShape(true)
	if y.HasShape() {
		t.Fatal("shared type flags")
	}
	if _, err := a.NewTuple(x, y); err == nil {
		t.Fatal("foreign type accepted")
	}
	if _, err := a.NewTuple(nil, x); err == nil {
		t.Fatal("nil type accepted")
	}
	pair, err := a.NewTuple(x, x)
	if err != nil {
		t.Fatal(err)
	}
	first, second, err := pair.TupleTypes()
	if err != nil || first != x || second != x || !pair.HasShape() || pair.Universe() != a {
		t.Fatal("upstream tuple construction lost identity/flags")
	}
	p, q := a.NewPkg("same/path", "p"), b.NewPkg("same/path", "p")
	if p == q || p.Universe() != a || q.Universe() != b {
		t.Fatal("package map shared")
	}
	buf := []byte("Name")
	sym := p.LookupBytes(buf)
	buf[0] = 'x'
	if sym.Name != "Name" || p.Lookup("Name") != sym || q.Lookup("Name") == sym || sym.Universe() != a {
		t.Fatal("symbol identity or input lifetime lost")
	}
	if got, existed := p.LookupOK("Name"); got != sym || !existed {
		t.Fatal("LookupOK semantics changed")
	}
	if _, err := a.Lookup(q, "bad"); err == nil {
		t.Fatal("foreign package accepted")
	}
	anon, err := a.Lookup(nil, "anon")
	if err != nil || anon.Universe() != a {
		t.Fatal("nil package lost explicit owner")
	}
	other, _ := b.Lookup(nil, "anon")
	if other == anon {
		t.Fatal("anonymous package shared")
	}
	callback := p.LookupBytes
	expr := (*types.Pkg).LookupNum
	if callback([]byte("Name")) != sym || expr(p, "v", 23).Name != "v23" {
		t.Fatal("receiver binding changed")
	}
	var native *gcbridge.Type = x
	var nativeSym *gcbridge.Sym = sym
	p.Pathsym = &gcbridge.LinkSymbol{Name: "p", Size: 17}
	if native != x || nativeSym != sym || p.Pathsym.Size != 17 {
		t.Fatal("native model copied")
	}
	_ = b.NewPkg("extra", "extra")
	if p.Lookup("Name") != sym || pair.Universe() != a {
		t.Fatal("later compilation invalidated result")
	}
}
func TestConcurrentUniverses(t *testing.T) {
	var wg sync.WaitGroup
	for i := 0; i < 24; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			u := universe(t, []string{"amd64", "arm64", "386"}[i%3])
			p := u.NewPkg("same", "p")
			for j := 0; j < 50; j++ {
				n := fmt.Sprint(j)
				s := p.LookupBytes([]byte(n))
				s.SetUniq(true)
				x, _ := u.Scalar(types.Int)
				if s.Universe() != u || x.Universe() != u || !s.Uniq() {
					t.Error("owner leak")
					return
				}
			}
		}(i)
	}
	wg.Wait()
}
func TestSymbolOrderAndInvalidEntry(t *testing.T) {
	u := universe(t, "arm64")
	a, b := u.NewPkg("a", "a"), u.NewPkg("b", "b")
	got := []*types.Sym{b.Lookup("z"), a.Lookup("z"), b.Lookup("A"), a.Lookup("Φ")}
	slices.SortFunc(got, types.CompareSyms)
	if got[0].Name != "A" || got[1].Name != "Φ" || got[2].Pkg != a || got[3].Pkg != b {
		t.Fatal("upstream symbol ordering changed")
	}
	if _, err := types.NewUniverse(target.Layout{}); err == nil {
		t.Fatal("zero layout accepted")
	}
	if _, err := u.Scalar(types.Tuple); err == nil {
		t.Fatal("non-scalar accepted")
	}
	t.Run("conflicting package name", func(t *testing.T) {
		defer func() {
			if recover() == nil {
				t.Error("conflicting names accepted")
			}
		}()
		u.NewPkg("a", "different")
	})
	t.Run("nil receiver", func(t *testing.T) {
		defer func() {
			if recover() == nil {
				t.Error("hidden nil-package fallback")
			}
		}()
		var p *types.Pkg
		p.Lookup("x")
	})
}

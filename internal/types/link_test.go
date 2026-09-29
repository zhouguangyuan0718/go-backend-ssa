package types

import (
	"go-backend-ssa/internal/cmd/obj"
	"sync"
	"testing"
)

func TestNativeSymbolBinding(t *testing.T) {
	a, b := NewUniverse(8, 8, 1<<50), NewUniverse(8, 8, 1<<50)
	left, right := &obj.Link{}, &obj.Link{}
	a.BindLink(left)
	a.BindLink(left)
	b.BindLink(right)
	for _, tc := range []struct{ path, name, want, pkg string }{
		{"example/p", "F", "example/p.F", "example/p"},
		{"go", "shape.int", "go:shape.int", "go"},
		{"type", "int", "type:int", "type"},
		{"example/p", "_", "_", "example/p"},
	} {
		x, y := a.NewPkg(tc.path, "p").Lookup(tc.name), b.NewPkg(tc.path, "p").Lookup(tc.name)
		s := x.Linksym()
		if s.Name != tc.want || s.Pkg != tc.pkg || s.ABI() != obj.ABI0 || s != left.Lookup(s.Name) || s == y.Linksym() {
			t.Fatalf("native symbol identity: %+v", s)
		}
		x.SetFunc(true)
		fn := x.Linksym()
		if fn == s || fn.ABI() != obj.ABIInternal || fn != x.LinksymABI(obj.ABIInternal) {
			t.Fatal("function ABI separation")
		}
	}
	renamed := a.NewPkg("example/p", "p").Lookup("Local")
	renamed.Linkname = "runtime.external"
	if s := renamed.Linksym(); s.Name != "runtime.external" || s.Pkg != "_" || s != left.Lookup("runtime.external") {
		t.Fatal("native linkname")
	}
	mustPanic := func(f func()) {
		t.Helper()
		defer func() {
			if recover() == nil {
				t.Error("invalid binding accepted")
			}
		}()
		f()
	}
	mustPanic(func() { a.BindLink(right) })
	mustPanic(func() { NewUniverse(8, 8, 1<<50).NewPkg("p", "p").Lookup("X").Linksym() })
	mustPanic(func() { a.BindLink(nil) })
}

func TestConcurrentNativeSymbolNamespaces(t *testing.T) {
	var wg sync.WaitGroup
	symbols := make([]*obj.LSym, 16)
	for i := range symbols {
		wg.Go(func() {
			u := NewUniverse(8, 8, 1<<50)
			u.BindLink(&obj.Link{})
			s := u.NewPkg("same/path", "p").Lookup("F")
			s.SetFunc(true)
			symbols[i] = s.Linksym()
			if symbols[i] != s.Linksym() {
				t.Error("unstable native lookup")
			}
		})
	}
	wg.Wait()
	for i, s := range symbols {
		for _, other := range symbols[:i] {
			if s == other {
				t.Fatal("shared compiler symbol")
			}
		}
	}
}

package noder

import (
	"bytes"
	"go-backend-ssa/internal/syntax"
	"go-backend-ssa/internal/types2"
	"sync"
	"testing"
)

const rangeSource = `package p
func Each(yield func(int)bool){for i:=0;i<4;i++{if !yield(i){return}}}
func F()(n int){for x:=range Each{defer func(){n++}();if x==2{break};n+=x};return}
`

func rewriteAndWrite(t *testing.T, c *Context) (*types2.Package, *types2.Info, []*syntax.File, string) {
	t.Helper()
	pkg, info, files := checked(t, c.Checker, rangeSource)
	old := pkg.Scope().Lookup("F").Type()
	closures, err := c.RewriteRangeFuncs(pkg, info, files)
	if err != nil {
		t.Fatal(err)
	}
	if len(closures) != 1 {
		t.Fatalf("range body markers: %d", len(closures))
	}
	for closure, body := range closures {
		if !body || closure.GetTypeInfo().Type == nil {
			t.Fatal("untyped range closure")
		}
	}
	syntax.Inspect(files[0], func(n syntax.Node) bool {
		if r, ok := n.(*syntax.RangeClause); ok {
			if _, ok := c.Checker.CoreType(r.X.GetTypeInfo().Type).(*types2.Signature); ok {
				t.Error("function range not lowered")
			}
		}
		return true
	})
	data, err := c.WritePackage(pkg, info, files, closures)
	if err != nil || data == "" {
		t.Fatalf("range writer: %v", err)
	}
	if pkg.Scope().Lookup("F").Type() != old {
		t.Fatal("old signature lost")
	}
	again, err := c.WritePackage(pkg, info, files, closures)
	if err != nil || again != data {
		t.Fatalf("repeat writer: %v", err)
	}
	return pkg, info, files, data
}

func runtimeHelper(t *testing.T, info *types2.Info) *types2.Package {
	t.Helper()
	var pkg *types2.Package
	names := map[string]bool{}
	for name, obj := range info.Uses {
		if name.GetTypeInfo().IsRuntimeHelper() {
			if pkg != nil && obj.Pkg() != pkg {
				t.Fatal("split runtime helper package")
			}
			pkg = obj.Pkg()
			names[obj.Name()] = true
		}
	}
	if pkg == nil || !names["panicrangestate"] || !names["deferrangefunc"] {
		t.Fatalf("missing runtime helpers: %v", names)
	}
	return pkg
}

func TestRangeFuncIsolation(t *testing.T) {
	var wg sync.WaitGroup
	for range 12 {
		wg.Go(func() {
			c := writerContext(t)
			pkg, info, _, data := rewriteAndWrite(t, c)
			helper := runtimeHelper(t, info)
			if helper.Context() != c.Checker {
				t.Error("helper belongs to another checker")
			}
			old := helper.Scope().Lookup("panicrangestate").Type()
			_, next, _, nextData := rewriteAndWrite(t, c)
			if runtimeHelper(t, next) == helper || helper.Scope().Lookup("panicrangestate").Type() != old || pkg.Scope().Lookup("F") == nil || data != nextData {
				t.Error("old graphs invalidated or output changed")
			}
		})
	}
	wg.Wait()
}

func TestRangeFuncChecksAndBoundaries(t *testing.T) {
	c := writerContext(t)
	if c.Debug.RangeFuncCheck != 1 {
		t.Fatal("native default checks disabled")
	}
	_, _, _, checkedData := rewriteAndWrite(t, c)
	c.Debug.RangeFuncCheck = 0
	pkg, info, files := checked(t, c.Checker, rangeSource)
	closures, err := c.RewriteRangeFuncs(pkg, info, files)
	if err != nil {
		t.Fatal(err)
	}
	uncheckedData, err := c.WritePackage(pkg, info, files, closures)
	if err != nil || checkedData == uncheckedData {
		t.Fatalf("explicit misuse check flag: %v", err)
	}
	foreign := writerContext(t)
	if _, err := foreign.RewriteRangeFuncs(pkg, info, files); err == nil {
		t.Fatal("foreign checker accepted")
	}
	if _, err := c.RewriteRangeFuncs(pkg, nil, files); err == nil {
		t.Fatal("missing info accepted")
	}
	var trace bytes.Buffer
	c.Flag.W = 1
	c.Typecheck.IR.TraceWriter = &trace
	rewriteAndWrite(t, c)
	if trace.Len() == 0 {
		t.Fatal("native AST dump not routed to instance writer")
	}
}

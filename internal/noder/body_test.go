package noder

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"go-backend-ssa/internal/go/pkgbits"
	"go-backend-ssa/internal/ir"
	"go-backend-ssa/internal/objw"
	"go-backend-ssa/internal/reflectdata"
	"go-backend-ssa/internal/rttype"
	"go-backend-ssa/internal/staticdata"
	"go-backend-ssa/internal/types"
	"sync"
	"testing"
)

// This test drives individual native reader operations. The production package
// driver and its inlining phase have not been migrated yet.
func drainTestBodies(c *Context) {
	for len(c.todoDicts) > 0 || len(c.todoBodies) > 0 {
		if len(c.todoDicts) > 0 {
			i := len(c.todoDicts) - 1
			f := c.todoDicts[i]
			c.todoDicts = c.todoDicts[:i]
			f()
			continue
		}
		i := len(c.todoBodies) - 1
		fn := c.todoBodies[i]
		c.todoBodies = c.todoBodies[:i]
		pri := c.bodyReader[fn]
		pri.funcBody(c, fn)
		if fn.OClosure == nil && len(pri.dict.targs) != 0 {
			c.Typecheck.Target.Funcs = append(c.Typecheck.Target.Funcs, fn)
		}
	}
}
func bodyContext(t *testing.T) *Context {
	c := writerContext(t)
	c.Ctxt.Arch.ByteOrder = binary.LittleEndian
	c.Ctxt.Pkgpath = "example/p"
	c.Typecheck.InitRuntime()
	c.Flag.Dwarf = true
	ow := objw.NewContext(c.Types, c.Ctxt, "linux")
	rt := rttype.NewContext(ow)
	rt.Init()
	c.ReflectData = reflectdata.NewContext(c.Typecheck, rt, staticdata.NewContext(c.Typecheck.IR, ow))
	return c
}
func readTestSource(t *testing.T, c *Context, source string, names ...string) map[string]*ir.Name {
	t.Helper()
	pkg, info, files := checked(t, c.Checker, source)
	closures, err := c.RewriteRangeFuncs(pkg, info, files)
	if err != nil {
		t.Fatal(err)
	}
	data, err := c.WritePackage(pkg, info, files, closures)
	if err != nil {
		t.Fatal(err)
	}
	pr := c.newPkgReader(pkgbits.NewPkgDecoder(pkg.Path(), data))
	c.localPkgReader = pr
	found := map[string]*ir.Name{}
	for i := range pr.NumElems(pkgbits.SectionName) {
		r := pr.newReader(pkgbits.SectionName, index(i), pkgbits.SyncObject1)
		_, sym := r.qualifiedIdent()
		tag := pkgbits.CodeObj(r.Code(pkgbits.SyncCodeObj))
		if tag == pkgbits.ObjStub || sym.Pkg != c.Types.LocalPkg {
			continue
		}
		for _, name := range names {
			if sym.Name == name {
				n := pr.objIdx(index(i), nil, nil, false).(*ir.Name)
				found[name] = n
				if n.Class == ir.PFUNC {
					c.Typecheck.Target.Funcs = append(c.Typecheck.Target.Funcs, n.Func)
				}
			}
		}
	}
	if len(found) != len(names) {
		t.Fatal("missing declarations", found, names)
	}
	drainTestBodies(c)
	if c.Diagnostics.HasErrors() {
		t.Fatal(c.Diagnostics.Diagnostics())
	}
	return found
}
func TestNativeReaderTypesAndBodies(t *testing.T) {
	c := bodyContext(t)
	objects := readTestSource(t, c, `package p
 type RecF=func(RecI)
 type RecI interface{M(RecF)}
 type Node struct{Next *Node;Value int}
 type Alias=Node
 const Rounded float32=1.234567890123
 func Add(x,y int)int{for i:=0;i<y;i++ {if i%2==0{x+=i}};return x}
 func Closure(x int)func()int {return func()int{x++;return x}}
 func Id[T any](v T)T{return v}
 type Box[T any]struct{V T}
 func(b Box[T])Get()T{return b.V}
 func Generic()int{b:=Id(Box[int]{V:3});f:=b.Get;return f()}
 func Control(ch chan int,a any)int{defer close(ch);select{case v:=<-ch:return v;default:};switch x:=a.(type){case int:return x;case string:return len(x)};return 0}
 func Each(yield func(int)bool){yield(1);yield(2)}
 func Range()int{s:=0;for v:=range Each {s+=v};return s}
 `, "RecF", "RecI", "Node", "Alias", "Rounded", "Add", "Closure", "Generic", "Control", "Range")
	node := objects["Node"].Type()
	if node.Field(0).Type.Elem() != node || objects["Alias"].Type() != node || !objects["Alias"].Alias() {
		t.Fatal("recursive/alias identity")
	}
	if objects["Rounded"].Type() != c.Types.Types[types.TFLOAT32] || objects["Rounded"].Val().ExactString() == "1234567890123/1000000000000" {
		t.Fatal("constant was not rounded")
	}
	closures := 0
	for _, fn := range c.Typecheck.Target.Funcs {
		if fn.ClosureParent == objects["Closure"].Func && len(fn.Body) > 0 && len(fn.ClosureVars) == 1 {
			closures++
		}
	}
	if closures != 1 {
		t.Fatal("closure body not read immediately")
	}
	if len(c.Typecheck.Target.Funcs) < 10 || len(c.Ctxt.Data) == 0 || len(c.needWrapperTypes) == 0 {
		t.Fatal("instantiations/dictionaries/wrapper registration missing")
	}
	for _, fn := range c.Typecheck.Target.Funcs {
		if fn.Typecheck() != 1 || len(fn.Body) == 0 || fn.Body[0].Typecheck() != 1 {
			t.Fatalf("incomplete native body %v", fn)
		}
	}
	if len(objects["Add"].Func.Parents) == 0 {
		t.Fatal("native lexical scopes missing")
	}
	if c.Typecheck.IR.CurFunc != nil {
		t.Fatal("current function leaked")
	}
}

func TestConcurrentNativeReaderRetainsBodies(t *testing.T) {
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			c := bodyContext(t)
			old := readTestSource(t, c, `package p;func Id[T any](v T)T{return v};func F(v int)func()int{return func()int{return Id(v)}};func init(){}`, "F", "init")
			fn := old["F"].Func
			node := fn.Body[0]
			before := fmt.Sprint(node)
			name := old["init"].Sym().Name
			dictData := append([]byte(nil), c.Ctxt.Data[0].P...)
			other := bodyContext(t)
			fresh := readTestSource(t, other, `package p;func F(v string)string{return v};func init(){}`, "F", "init")
			if node != fn.Body[0] || fmt.Sprint(node) != before || old["F"].Sym() == fresh["F"].Sym() || name != "init.0" || fresh["init"].Sym().Name != "init.0" || !bytes.Equal(dictData, c.Ctxt.Data[0].P) {
				t.Error("retained body/dictionary/init name changed")
			}
			if c.Typecheck.IR.CurFunc != nil || len(c.todoBodies) != 0 || len(c.todoDicts) != 0 {
				t.Error("reader work or function state leaked")
			}
		})
	}
	wg.Wait()
}

package pkginit

import (
	"bytes"
	"fmt"
	"go-backend-ssa/internal/deadlocals"
	"go-backend-ssa/internal/escape"
	"go-backend-ssa/internal/ir"
	"go-backend-ssa/internal/slice"
	"go-backend-ssa/internal/ssagen"
	"go-backend-ssa/internal/walk"
	"strings"
	"sync"
	"testing"
)

const walkSource = `package p
var Counter int
var Sink any
var Global []int
func effect(n int)int{Counter++;return n}
func pair(n int)(int,int){return n,n+1}
func Eval(a []int,m map[int]int)int{a[effect(0)],m[effect(1)]=pair(effect(2));return effect(a[0])+effect(m[1])}
func CompareStrings(a,b string)bool{return a==b}
func CompareSmall(a,b struct{A int;B string;C any})bool{return a==b}
func CompareLarge(a,b [64]byte)bool{return a==b}
func Grow(n int)[]int{var s []int;for i:=0;i<n;i++{s=append(s,i)};return s}
func Capacity(n int)([]int,int){var s []int;for i:=0;i<n;i++{s=append(s,i)};return s,cap(s)}
func Map(m map[string]int,k string)(int,bool){m[k]++;v,ok:=m[k];delete(m,k);return v,ok}
func MapFat(m map[[128]byte][2048]byte,k [128]byte)[2048]byte{return m[k]}
func MapFloat(m map[float64]int,k float64){m[k]=m[k]+1}
func MakeMap(n int)map[string]int{return make(map[string]int,n)}
func MakeSlice(n int)[]int{s:=make([]int,n);copy(s,[]int{1,2});return append(s,3)}
func String(s string)([]byte,[]rune,string){b:=[]byte(s);r:=[]rune(s);return b,r,string(b)}
func RangeString(s string)int{n:=0;for i,r:=range s{n+=i+int(r)};return n}
func RangeMap(m map[int]int)int{n:=0;for k,v:=range m{n+=k+v};return n}
func RangeArray(a [4]int)int{n:=0;for _,v:=range a{n+=v};return n}
func RangeInt(n int)int{r:=0;for i:=range n{r+=i};return r}
func Channel(c chan int)(int,bool){select{case c<-1:return 1,true;case x,ok:=<-c:return x,ok;default:return 0,false}}
func ChanRange(c chan int)int{n:=0;for x:=range c{n+=x};return n}
func Deferred(f func(int),n int){defer f(n);go f(n+1)}
func Closure(n int)func()int{return func()int{n++;return n}}
func Literal()any{return "constant"}
func Nil()*int{return nil}
func Assert(x any)(int,bool){v,ok:=x.(int);return v,ok}
func SwitchInt(n int)int{switch n{case 0:return 4;case 1:return 2;case 2:return 9;case 3:return 7;case 4:return 1;case 5:return 8;case 6:return 3;case 7:return 6};return 0}
func SwitchString(s string)int{switch s{case "apple":return 1;case "pear":return 2;case "banana":return 3;case "plum":return 4;case "a":return 5;case "b":return 6};return 0}
type I interface{M()int}
type Number int
func(n Number)M()int{return int(n)}
func SwitchType(x any)int{switch v:=x.(type){case int:return v;case string:return len(v);case I:return v.M();case nil:return -1};return 0}
func Method(n Number)func()int{return n.M}
func Iface(n Number)I{return n}
func Generic[T comparable](m map[T]int,k T)int{return m[k]}
func Instance(m map[string]int,k string)int{return Generic(m,k)}
func Shape[T any](v T)any{return v}
func ShapeInstance(n int)any{return Shape(n)}
func Float(a,b float64)(float64,int64){return a/b,int64(a)}
func Division(a,b int)int{return a/b}
func Panic(v any){panic(v)}
`

func walkContext(t *testing.T, path string) *walk.Context {
	t.Helper()
	p := sourceContext(t, path, walkSource, false)
	p.MakeTask()
	tc := p.Noder.Typecheck
	ssagen.NewContext(tc).NewSymABIs().GenABIWrappers()
	deadlocals.NewContext(tc.IR).Funcs(tc.Target.Funcs)
	esc := escape.NewContext(tc, p.Noder.Inline.Log)
	esc.Funcs(tc.Target.Funcs)
	tc.IR.Debug.VariableMakeThreshold = 64
	slice.NewContext(esc, p.Noder.ReflectData).Funcs(tc.Target.Funcs)
	for _, fn := range walkFunctions(tc.Target.Funcs) {
		tc.IR.InitLSym(fn, true)
		tc.IR.Types.CalcSize(fn.Type())
	}
	return walk.NewContext(p.Noder, esc)
}
func walkFunctions(roots []*ir.Func) []*ir.Func {
	var result []*ir.Func
	seen := map[*ir.Func]bool{}
	for _, root := range roots {
		todo := []*ir.Func{root}
		for len(todo) > 0 {
			fn := todo[len(todo)-1]
			todo = todo[:len(todo)-1]
			if seen[fn] {
				continue
			}
			seen[fn] = true
			result = append(result, fn)
			todo = append(todo, fn.Closures...)
		}
	}
	return result
}
func walkDump(c *walk.Context) string {
	var b bytes.Buffer
	for _, fn := range walkFunctions(c.Typecheck.Target.Funcs) {
		ir.FDump(&b, "function", fn)
	}
	return b.String()
}
func TestNativeWalk(t *testing.T) {
	c := walkContext(t, "example/p")
	before := walkDump(c)
	for _, fn := range walkFunctions(c.Typecheck.Target.Funcs) {
		c.Walk(fn)
		if c.Typecheck.IR.CurFunc != fn {
			t.Fatal("native current function contract")
		}
	}
	after := walkDump(c)
	if before == after {
		t.Fatal("walk did not rewrite IR")
	}
	for _, name := range []string{"mapaccess", "selectgo", "convT", "memequal"} {
		if !strings.Contains(after, name) {
			t.Errorf("missing lowered operation %s", name)
		}
	}
}
func TestWalkIsolation(t *testing.T) {
	old := walkContext(t, "example/p")
	for _, fn := range walkFunctions(old.Typecheck.Target.Funcs) {
		old.Walk(fn)
	}
	before := walkDump(old)
	var wg sync.WaitGroup
	for i := range 8 {
		wg.Go(func() {
			c := walkContext(t, fmt.Sprintf("example/p%d", i))
			for _, fn := range walkFunctions(c.Typecheck.Target.Funcs) {
				c.Walk(fn)
			}
			if walkDump(old) != before {
				t.Error("retained graph changed")
			}
		})
	}
	wg.Wait()
}
func TestWalkRejectsForeignFunction(t *testing.T) {
	a, b := walkContext(t, "example/a"), walkContext(t, "example/b")
	fn := b.Typecheck.Target.Funcs[0]
	before := walkDump(b)
	defer func() {
		if recover() == nil {
			t.Error("foreign function accepted")
		}
		if a.Typecheck.IR.CurFunc != nil || walkDump(b) != before {
			t.Error("mutated before ownership rejection")
		}
	}()
	a.Walk(fn)
}

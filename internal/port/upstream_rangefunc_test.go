package port

// These cases pass through the native checker, the complete native rangefunc
// rewrite, and the complete native writer on both sides. The reference keeps
// original package globals and initializers; only its imports are relocated.
const rangefuncByteTests = `
func TestNativeRangeFuncBytes(t *testing.T){
 const prefix="package p;func Each(y func(int)bool){for i:=0;i<4;i++{if !y(i){return}}};"
 sources:=[]string{
  prefix+"func F()(n int){for x:=range Each{n+=x};return}",
  prefix+"func F()(n int){for x:=range Each{if x==1{continue};if x==3{break};n+=x};return}",
  prefix+"func F()(n int){Outer:for x:=range Each{for y:=range Each{if y==1{continue Outer};if x==2{break Outer};n+=x+y}};return}",
  prefix+"func F()(n int){for x:=range Each{for y:=range Each{n+=x+y;if y==2{goto Done}}};Done:return}",
  prefix+"func F()int{for x:=range Each{for y:=range Each{if x+y==3{return x*y}}};return -1}",
  prefix+"func F()(n int){for x:=range Each{defer func(){n+=x}()};return}",
  prefix+"func F()[]func()int{var fs []func()int;for x:=range Each{fs=append(fs,func()int{return x})};return fs}",
  "package p;func Pair(y func(int,string)bool){y(1,\"a\")};func F()(n int,s string){for n,s=range Pair{};for k,v:=range Pair{n+=k;s+=v};return}",
  "package p;func Zero(y func()bool){y()};func F()(n int){for range Zero{n++};return}",
  "package p;func Values[T any](xs []T)func(func(T)bool){return func(y func(T)bool){for _,x:=range xs{if !y(x){return}}}};func F[T ~int](xs []T)(n T){for x:=range Values(xs){n+=x};return};var X=F([]int{1,2})",
  prefix+"func F()func()int{return func()(n int){for x:=range Each{n+=x};return}}",
  prefix+"func F()(n int){defer func(){recover()}();for x:=range Each{defer func(){n++}();if x==2{panic(x)};n+=x};return}",
 }
 for i,source:=range sources{for _,arch:=range []string{"386","amd64"}{for _,frames:=range []int{-1,0}{for _,checks:=range []int{0,1}{t.Run(fmt.Sprintf("%d/%s/%d/%d",i,arch,frames,checks),func(t *testing.T){want,got:=writeReference(t,source,arch,frames,checks),writePorted(t,source,arch,frames,checks);if got!=want{at:=0;for at<len(got)&&at<len(want)&&got[at]==want[at]{at++};t.Fatalf("rangefunc stub differs: offset %d, ported %d bytes, reference %d bytes",at,len(got),len(want))}})}}}}
}
`

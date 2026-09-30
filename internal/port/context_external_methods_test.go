package port

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestExternalContextValueMethods(t *testing.T) {
	const hostPath = "src/cmd/compile/internal/host/host.go"
	const callerPath = "src/cmd/compile/internal/p/p.go"
	const host = `package host
 var names=[2]string{"zero","one"}
 var next Code
 type Code int
 type Cell struct{N int}
 func(c Code)String()string{return names[c]}
 func(c Code)Next()Code{return next}
 func(c Code)Recover(out *any){_ = names[c];*out=recover()}
 func(c *Cell)Add(xs ...int)Code {for _,v:=range xs {c.N+=v};return Code(c.N+len(names[0]))}
 `
	const caller = `package p
 import h "cmd/compile/internal/host"
 func Direct(o h.Code)string{return o.String()}
 func Catch()(out any){defer h.Code(0).Recover(&out);panic("kept")}
 func Bound(o h.Code)func()string{return o.String}
 func Expression()func(h.Code)string{return h.Code.String}
 func UseExpression(o h.Code)string{return h.Code.String(o)}
 func Nested(o h.Code)func()string{return o.Next().String}
 func Pointer(v []h.Cell)func(...int)h.Code{return v[0].Add}
 func PointerExpression()func(*h.Cell,...int)h.Code{return (*h.Cell).Add}
 `
	repo, pin := fixtureRepo(t, map[string]string{"LICENSE": "fixture", "PATENTS": "fixture", hostPath: host, callerPath: caller})
	source, err := OpenSource(context.Background(), repo, pin)
	if err != nil {
		t.Fatal(err)
	}
	plan := Plan{InspectRoots: []string{"cmd/compile/internal/p"}, Units: []Unit{
		{Source: hostPath, WholeFile: true, Context: &ContextRule{Type: "Context", Constructor: "NewContext", Selective: true, Adapter: "adapter_context.go", Fields: map[string]string{"names": "Names", "next": "NextCode"}}, ContextParamExports: []string{"Code.String", "Code.Next", "Cell.Add", "Code.Recover"}},
		{Source: callerPath, WholeFile: true, Context: &ContextRule{Type: "Context", Constructor: "NewContext", Selective: true, Adapter: "adapter_context.go"}, ContextParamCalls: []ContextParamCallRule{
			{Source: hostPath, Method: "Code.String", Field: "Right", ContextType: "Context"},
			{Source: hostPath, Method: "Code.Next", Field: "Left", ContextType: "Context"},
			{Source: hostPath, Method: "Cell.Add", Field: "Right", ContextType: "Context"},
			{Source: hostPath, Method: "Code.Recover", Field: "Right", ContextType: "Context"},
		}},
	}}
	dir := t.TempDir()
	writeTestFile(t, dir, "internal/host/adapter_context.go", `package host;type Context struct{Names [2]string;NextCode Code}`)
	writeTestFile(t, dir, "internal/p/adapter_context.go", `package p;import "go-backend-ssa/internal/host";type Context struct{Left,Right *host.Context}`)
	r, err := Render(source, plan, dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := Install(dir, r); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, dir, "go.mod", "module go-backend-ssa\n\ngo 1.27.1\n")
	writeTestFile(t, dir, "internal/p/adapter_test.go", `package p
 import("testing";"go-backend-ssa/internal/host")
 func TestBindings(t *testing.T){
  a,b:=host.NewContext(),host.NewContext();a.NextCode=1;b.Names=[2]string{"B","B1"};p:=NewContext();p.Left=a;p.Right=b
  if p.Catch()!="kept"{t.Fatal("direct deferred recovery")};direct,bound,expr,nested:=p.Direct(0),p.Bound(0),p.Expression(),p.Nested(0)
  if direct!="B"||p.UseExpression(1)!="B1" {t.Fatal("direct calls")}
  cells:=[]host.Cell{{N:1}};pointer:=p.Pointer(cells);pointerExpr:=p.PointerExpression()
  cells[0].N=2;p.Right=a;a.NextCode=0
  if bound()!="B"||expr(1)!="B1"||nested()!="B1"{t.Fatal("receiver/context not captured")}
  if pointer(3,4)!=10||cells[0].N!=9||pointerExpr(&cells[0],1)!=11 {t.Fatal("variadic pointer receiver")}
 }
 `)
	cmd := exec.Command("go", "test", "-race", "./...")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOWORK=off", "GOTOOLCHAIN=go1.27.1")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	source.cache[callerPath] = []byte(caller + "\nfunc Bad()any{return h.Code(0).Recover}\n")
	if _, err := Render(source, plan, dir); err == nil || !strings.Contains(err.Error(), "direct recover") {
		t.Fatalf("external recover method value accepted: %v", err)
	}
	source.cache[callerPath] = []byte(caller)
	plan.Units[1].ContextParamCalls[0].ContextType = "Missing"
	if err := plan.Validate(); err == nil || !strings.Contains(err.Error(), "explicit export") {
		t.Fatalf("unreviewed target accepted: %v", err)
	}
	plan.Units[1].ContextParamCalls[0].ContextType = "Context"
	plan.Units[1].ContextParamCalls = append(plan.Units[1].ContextParamCalls, plan.Units[1].ContextParamCalls[0])
	if err := plan.Validate(); err == nil {
		t.Fatal("duplicate method accepted")
	}
}

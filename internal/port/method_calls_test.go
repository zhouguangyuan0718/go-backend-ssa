package port

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestBoundNativeMethodCalls(t *testing.T) {
	const hostPath = "src/cmd/compile/internal/host/host.go"
	const callerPath = "src/cmd/compile/internal/p/p.go"
	const otherPath = "src/cmd/compile/internal/p/z.go"
	const host = `package host
type Cell struct{N int}
func(c *Cell)Value(xs ...int)int{if c==nil{return -1};v:=c.N;for _,x:=range xs{v+=x};return v}
func(c *Cell)Catch(){recover()}
`
	const caller = `package p
import h "cmd/compile/internal/host"
var count int
func side(p *h.Cell)*h.Cell{count++;return p}
func Run(p *h.Cell,xs ...int)int{return side(p).Value(xs...)}
`
	repo, pin := fixtureRepo(t, map[string]string{"LICENSE": "fixture", "PATENTS": "fixture", hostPath: host, callerPath: caller, otherPath: "package p"})
	source, e := OpenSource(context.Background(), repo, pin)
	if e != nil {
		t.Fatal(e)
	}
	plan := Plan{InspectRoots: []string{"cmd/compile/internal/p"}, Units: []Unit{
		{Source: hostPath, WholeFile: true},
		{Source: callerPath, WholeFile: true, Context: &ContextRule{Type: "Context", Constructor: "NewContext", Selective: true, Adapter: "adapter_context.go", Fields: map[string]string{"count": "Count"}}, MethodCalls: []MethodCallRule{{Source: hostPath, Method: "Cell.Value", Adapter: "bound"}}},
	}}
	plan.Units = append(plan.Units, Unit{Source: otherPath, WholeFile: true, Context: &ContextRule{Type: "Context", Constructor: "NewContext", Selective: true, Adapter: "adapter_context.go"}})
	dir := t.TempDir()
	const adapter = `package p
import "go-backend-ssa/internal/host"
type Context struct{Count,Bound,Fallback int}
func(c *Context)bound(p *host.Cell,xs ...int)int{c.Bound++;if p==nil{return c.Fallback};return p.Value(xs...)}
`
	writeTestFile(t, dir, "internal/p/adapter_context.go", adapter)
	r, e := Render(source, plan, dir)
	if e != nil {
		t.Fatal(e)
	}
	recorded := false
	for _, entry := range r.Ledger.Files {
		for _, decl := range entry.Declarations {
			if decl == "method-call:Cell.Value:bound" && entry.SignatureSources[hostPath] != "" {
				recorded = true
			}
		}
	}
	if !recorded {
		t.Fatal("missing whole-file method binding provenance")
	}
	if e = Install(dir, r); e != nil {
		t.Fatal(e)
	}
	writeTestFile(t, dir, "go.mod", "module go-backend-ssa\n\ngo 1.27.1\n")
	writeTestFile(t, dir, "internal/p/adapter_test.go", `package p
import("testing";"go-backend-ssa/internal/host")
func TestBoundary(t *testing.T){
 a,b:=NewContext(),NewContext();a.Fallback=17;b.Fallback=23
 if a.Run(nil)!=17||b.Run(nil)!=23||a.Run(&host.Cell{N:3},4,5)!=12{t.Fatal("receiver or arguments changed")}
 if a.Count!=2||a.Bound!=2||b.Count!=1||b.Bound!=1{t.Fatal("receiver evaluated twice or context leaked")}
}
`)
	cmd := exec.Command("go", "test", "-race", "./...")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOWORK=off", "GOTOOLCHAIN=go1.27.1")
	if out, e := cmd.CombinedOutput(); e != nil {
		t.Fatalf("%v\n%s", e, out)
	}
	for name, extra := range map[string]string{
		"value":            "func Bad(p *h.Cell)any{return p.Value}",
		"expression":       "func Bad(p *h.Cell)int{return (*h.Cell).Value(p)}",
		"implicit address": "func Bad(p h.Cell)int{return p.Value()}",
		"promoted":         "type Embedded struct{*h.Cell};func Bad(p *Embedded)int{return p.Value()}",
	} {
		t.Run(name, func(t *testing.T) {
			source.cache[callerPath] = []byte(caller + extra)
			if _, e := Render(source, plan, dir); e == nil || !strings.Contains(e.Error(), "direct exact receiver") {
				t.Fatalf("unsafe method binding accepted: %v", e)
			}
		})
	}
	source.cache[callerPath] = []byte(caller)
	source.cache[otherPath] = []byte(`package p;import h "cmd/compile/internal/host";func Bad(p *h.Cell)any{return p.Value}`)
	if _, e := Render(source, plan, dir); e == nil || !strings.Contains(e.Error(), "direct exact receiver") {
		t.Fatalf("unreviewed method use in another file accepted: %v", e)
	}
	source.cache[otherPath] = []byte("package p")
	writeTestFile(t, dir, "internal/p/adapter_context.go", strings.Replace(adapter, "bound(p *host.Cell,xs ...int)", "bound(p *host.Cell,xs []int)", 1))
	if _, e = Render(source, plan, dir); e == nil {
		t.Fatal("changed variadic signature accepted")
	}
	writeTestFile(t, dir, "internal/p/adapter_context.go", adapter)
	plan.Units[1].MethodCalls[0].Source = "src/cmd/compile/internal/host/missing.go"
	if _, e = Render(source, plan, dir); e == nil || !strings.Contains(e.Error(), "source mismatch") {
		t.Fatalf("wrong source accepted: %v", e)
	}
	plan.Units[1].MethodCalls[0].Source = hostPath
	plan.Units[1].MethodCalls[0].Method = "Cell.Catch"
	if _, e = Render(source, plan, dir); e == nil || !strings.Contains(e.Error(), "unsupported method") {
		t.Fatalf("recover method accepted: %v", e)
	}
	plan.Units[1].MethodCalls[0].Method = "Cell.Value"
	source.cache[callerPath] = []byte(`package p;var count int;func Run()int{return count}`)
	if _, e = Render(source, plan, dir); e == nil || !strings.Contains(e.Error(), "unused") {
		t.Fatalf("unused binding accepted: %v", e)
	}
}

package port

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestEmbeddedOwnerConstructionAndCopies(t *testing.T) {
	const base = `package p
 type miniNode struct{Pos int}
 var calls int
 func(n *miniNode)Call()int{calls++;return calls}
 `
	const expr = `package p
 type miniExpr struct{miniNode;Type int}
 type Expr struct{miniExpr;Value int}
 func New()*Expr{return new(Expr)}
 func Literal()*Expr{return &Expr{Value:3}}
 func Full()*Expr{return &Expr{miniExpr:miniExpr{miniNode:miniNode{Pos:7},Type:8},Value:9}}
 func Unkeyed()*Expr{return &Expr{miniExpr{miniNode{7},8},9}}
 func Zero()Expr{var n Expr;return n}
 func(n *Expr)Clone()*Expr{c:=*n;return &c}
 func(n *Expr)Make()*Expr{return &Expr{Value:n.Value}}
 func(n *Expr)Counter()int{return calls}
 func(n *Expr)Callback()func()int{return n.Call}
 func Describe(n *Expr)int{calls++;return n.Value}
 type Nodes []*Expr
 func(l Nodes)Count()int{calls++;return len(l)}
 func Observe(n *Expr)int{return Describe(n)}
 `
	repo, pin := fixtureRepo(t, map[string]string{"LICENSE": "fixture", "PATENTS": "fixture", "src/cmd/compile/internal/p/base.go": base, "src/cmd/compile/internal/p/expr.go": expr})
	s, err := OpenSource(context.Background(), repo, pin)
	if err != nil {
		t.Fatal(err)
	}
	ctx := func() *ContextRule {
		return &ContextRule{Type: "Context", Constructor: "NewContext", Adapter: "adapter_context.go", Selective: true}
	}
	bc := ctx()
	bc.Fields = map[string]string{"calls": "calls"}
	plan := Plan{InspectRoots: []string{"cmd/compile/internal/p"}, Units: []Unit{
		{Source: "src/cmd/compile/internal/p/base.go", WholeFile: true, Context: bc, Owner: &OwnerRule{Types: []string{"miniNode"}, Context: "Context", Field: "compilation"}},
		{Source: "src/cmd/compile/internal/p/expr.go", WholeFile: true, Context: ctx(), ContextFrom: map[string]ContextSource{"Describe": {Argument: "n", Resolver: "nodeContext"}, "Nodes.Count": {Argument: "l", Resolver: "listContext"}}, Owner: &OwnerRule{Types: []string{"miniExpr", "Expr"}, Context: "Context", Field: "compilation", Embedded: map[string]string{"miniExpr": "miniNode", "Expr": "miniExpr.miniNode"}}},
	}}
	dir := t.TempDir()
	writeTestFile(t, dir, "internal/p/adapter_context.go", `package p
 type Context struct{calls int}
 func nodeContext(n *Expr)*Context{if n==nil{return nil};return n.compilation}
 func listContext(l Nodes)*Context{if len(l)==0{return NewContext()};return l[0].compilation}
 `)
	r, err := Render(s, plan, dir)
	if err != nil {
		t.Fatal(err)
	}
	text := string(r.Files[plan.Units[1].output()])
	if strings.Count(text, "compilation *Context") != 0 {
		t.Fatal("duplicate ownership field in derived structs")
	}
	if err := Install(dir, r); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, dir, "go.mod", "module go-backend-ssa\n\ngo 1.27.1\n")
	writeTestFile(t, dir, "internal/p/owner_test.go", `package p
 import("testing";"sync")
 func TestEmbedded(t *testing.T){var wg sync.WaitGroup;for range 16{wg.Go(func(){a,b:=NewContext(),NewContext();zero:=a.Zero();for _,n:=range []*Expr{a.New(),a.Literal(),a.Full(),a.Unkeyed(),&zero}{if n.compilation!=a{t.Error("missing inherited owner")};clone:=n.Clone();fresh:=n.Make();if clone.compilation!=a||fresh.compilation!=a||clone.Value!=n.Value{t.Error("lost copy owner")};callback:=n.Callback();old:=a.calls;if callback()!=old+1||n.Counter()!=old+1||b.calls!=0{t.Error("foreign callback state")}};n:=a.Literal();before:=a.calls;var describe func(*Expr)int=Describe;var observer interface{Count()int}=Nodes{n};if describe(n)!=3||Observe(n)!=3||observer.Count()!=1||a.calls!=before+3||b.calls!=0{t.Error("native signature or context resolver")};if n:=a.Unkeyed();n.Pos!=7||n.Type!=8||n.Value!=9{t.Error("unkeyed initialization")};defer func(){if recover()==nil{t.Error("zero owner accepted")}}();new(Expr).Counter()})};wg.Wait()}
 `)
	cmd := exec.Command("go", "test", "-race", "./...")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOWORK=off", "GOTOOLCHAIN=go1.27.1")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	bindings := plan.Units[1].ContextFrom
	plan.Units[1].ContextFrom["Describe"] = ContextSource{Argument: "missing", Resolver: "nodeContext"}
	if _, err := Render(s, plan, dir); err == nil || !strings.Contains(err.Error(), "context source argument not found") {
		t.Fatalf("stale context argument: %v", err)
	}
	plan.Units[1].ContextFrom["Describe"] = ContextSource{Argument: "n", Resolver: "nodeContext"}
	for _, tc := range []struct{ name, source, path, want string }{
		{"missing path", expr, "miniExpr.missing", "not an embedded field"},
		{"non-owner leaf", expr, "miniExpr", "does not end at a declared owner"},
		{"pointer base", `package p;type miniExpr struct{*miniNode};type Expr struct{miniExpr}`, "miniExpr.miniNode", "local named value"},
		{"base expression", expr + "\nfunc CopyBase(e miniExpr)*Expr{return &Expr{miniExpr:e}}", "miniExpr.miniNode", "requires a literal"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s.cache[plan.Units[1].Source] = []byte(tc.source)
			plan.Units[1].ContextFrom = bindings
			if tc.name == "pointer base" {
				plan.Units[1].ContextFrom = nil
			}
			plan.Units[1].Owner.Embedded["Expr"] = tc.path
			if _, err := Render(s, plan, dir); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("want %q, got %v", tc.want, err)
			}
		})
	}
}

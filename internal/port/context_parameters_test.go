package port

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestContextParameters(t *testing.T) {
	const code = `package p
 type Context struct{}
 func NewContext()*Context{return &Context{}}
 var total int
 type terms []int
 func(x terms)sum(more ...int)int{for _,n:=range append(x,more...){total+=n};return total}
 func(x terms)norm()int{return x.sum(3)}
 func(x terms)length()int{return len(x)}
 func(x terms)unnamed(int)int{return total}
 func PureCallback()func()int{return terms{1}.length}
 func Unnamed()int{return terms{}.unnamed(1)}
 type checker struct{n int}
 func(c *checker)check()int{if c==nil{return total};return c.n+total}
 func Bound()int{return terms{1,2}.norm()}
 func Expression()int{return terms.sum(terms{4},5,6)}
 func Parenthesized()int{return (terms{7}.sum)(8)}
 func NilReceiver()int{return (*checker)(nil).check()}
 func Callback()func()int{return func()int{return terms{9}.norm()}}
 func Pure()int{return len(terms{1})}
 `
	repo, pin := fixtureRepo(t, map[string]string{"LICENSE": "fixture", "PATENTS": "fixture", "src/cmd/compile/internal/p/p.go": code})
	s, err := OpenSource(context.Background(), repo, pin)
	if err != nil {
		t.Fatal(err)
	}
	plan := Plan{InspectRoots: []string{"cmd/compile/internal/p"}, Units: []Unit{{Source: "src/cmd/compile/internal/p/p.go", WholeFile: true, ContextParams: []string{"terms.sum", "terms.norm", "checker.check", "terms.length", "terms.unnamed"}, Context: &ContextRule{Existing: true, Selective: true, Type: "Context", Constructor: "NewContext", Fields: map[string]string{"total": "total"}}}}}
	dir := t.TempDir()
	r, err := Render(s, plan, dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := Install(dir, r); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, dir, "go.mod", "module go-backend-ssa\n\ngo 1.27.1\n")
	writeTestFile(t, dir, "internal/p/adapter_test.go", `package p
 import("testing";"sync")
 func TestBindings(t *testing.T){a,b:=NewContext(),NewContext();if a.Bound()!=6||a.Expression()!=21||a.Parenthesized()!=36||a.NilReceiver()!=36||a.Callback()()!=48||b.NilReceiver()!=0||Pure()!=1||PureCallback()()!=1||a.Unnamed()!=48{t.Fatal("method semantics or binding")};var wg sync.WaitGroup;for range 12{wg.Go(func(){c:=NewContext();if c.Bound()!=6||c.NilReceiver()!=6{t.Error("shared state")}})};wg.Wait()}
 `)
	cmd := exec.Command("go", "test", "-race", "./...")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOWORK=off", "GOTOOLCHAIN=go1.27.1")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	s.cache[plan.Units[0].Source] = []byte(code + "\nfunc Bad()func(...int)int{return terms{1}.sum}\n")
	if _, err := Render(s, plan, t.TempDir()); err == nil || !strings.Contains(err.Error(), "method value") {
		t.Fatalf("method value must be rejected: %v", err)
	}
}

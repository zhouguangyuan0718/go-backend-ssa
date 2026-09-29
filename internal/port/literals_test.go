package port

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestOwnedPositionalAndElidedLiterals(t *testing.T) {
	const source = `package p
 type Pair struct{A,B int}
 var sequence []int
 func mark(n int)int{sequence=append(sequence,n);return n}
 var basics=[...]*Pair{0:{1,2},2:{A:3,B:4}}
 func Build()([]*Pair,[]Pair,map[int]*Pair){return []*Pair{{mark(1),mark(2)},nil},[]Pair{{mark(3),mark(4)}},map[int]*Pair{0:{mark(5),mark(6)}}}
 func(n *Pair)Children()[]*Pair{return []*Pair{{n.A,n.B}}}
 func Shadow()int{type Pair struct{X int};return []Pair{{9}}[0].X}
 `
	repo, pin := fixtureRepo(t, map[string]string{"LICENSE": "fixture", "PATENTS": "fixture", "src/cmd/compile/internal/p/p.go": source})
	s, err := OpenSource(context.Background(), repo, pin)
	if err != nil {
		t.Fatal(err)
	}
	plan := Plan{InspectRoots: []string{"cmd/compile/internal/p"}, Units: []Unit{{Source: "src/cmd/compile/internal/p/p.go", Functions: []string{"mark", "Build", "Shadow"}, Declarations: []string{"Pair", "Pair.Children"}, Owner: &OwnerRule{Types: []string{"Pair"}, Context: "Context", Field: "owner"}, Context: &ContextRule{Type: "Context", Constructor: "newContext", Adapter: "adapter_context.go", Fields: map[string]string{"sequence": "sequence", "basics": "basics"}}}}}
	dir := t.TempDir()
	writeTestFile(t, dir, "internal/p/adapter_context.go", `package p;type Context struct{sequence []int;basics [3]*Pair}`)
	r, err := Render(s, plan, dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := Install(dir, r); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, dir, "go.mod", "module go-backend-ssa\n\ngo 1.27.1\n")
	writeTestFile(t, dir, "internal/p/adapter_test.go", `package p
 import("testing";"reflect";"sync")
 func TestGraphs(t *testing.T){check:=func(){a,b:=newContext(),newContext();if a.basics[0].owner!=a||a.basics[2].owner!=a||a.basics[1]!=nil||a.basics[0]==b.basics[0]{t.Error("initializers")};p,v,m:=a.Build();if p[0].A!=1||p[0].B!=2||p[0].owner!=a||p[1]!=nil||v[0].owner!=a||m[0].owner!=a||!reflect.DeepEqual(a.sequence,[]int{1,2,3,4,5,6}){t.Error("literal ownership or evaluation order")};kids:=p[0].Children();if kids[0].owner!=a||kids[0].A!=1||kids[0].B!=2||a.Shadow()!=9{t.Error("method owner or shadowing")}};check();var wg sync.WaitGroup;for range 16{wg.Go(check)};wg.Wait()}
 `)
	cmd := exec.Command("go", "test", "-race", "./...")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOWORK=off", "GOTOOLCHAIN=go1.27.1")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	// A blank positional field may evaluate an expression. Do not discard it or
	// create an illegal keyed '_' field while adding the owner.
	s.cache[plan.Units[0].Source] = []byte(`package p
 type Pair struct{A int;_ int}
 var sequence []int
 func mark(n int)int{sequence=append(sequence,n);return n}
 var basics=[...]*Pair{{1,2}}
 func Build()*Pair{return &Pair{mark(1),mark(2)}}
 func(n *Pair)Children()[]*Pair{return []*Pair{{n.A,1}}}
 func Shadow()int{return 9}
 `)
	if _, err := Render(s, plan, dir); err == nil || !strings.Contains(err.Error(), "blank field") {
		t.Fatalf("blank field effects silently removed: %v", err)
	}
}

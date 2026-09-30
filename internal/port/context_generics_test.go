package port

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestReviewedStatelessGenericHelpers(t *testing.T) {
	source := `package p
 var count int
 func tracked()int{count++;return count}
 func one[T ~int8|~int64](v T)bool{return v!=0 && v&(v-1)==0}
 func pair[A ~int8,B ~int64](a A,b B)bool{return one(a)&&one[B](b)}
 func Apply(v int64)bool{count++;f:=one[int64];return f(v)&&pair(int8(2),v)}
 `
	repo, pin := fixtureRepo(t, map[string]string{"LICENSE": "fixture", "PATENTS": "fixture", "src/cmd/compile/internal/p/p.go": source})
	s, err := OpenSource(context.Background(), repo, pin)
	if err != nil {
		t.Fatal(err)
	}
	u := Unit{Source: "src/cmd/compile/internal/p/p.go", Functions: []string{"tracked", "one", "pair", "Apply"}, StatelessFunctions: []string{"one", "pair"}, Context: &ContextRule{Type: "Context", Constructor: "newContext", Adapter: "adapter_context.go", Selective: true, Fields: map[string]string{"count": "count"}}}
	plan := Plan{InspectRoots: []string{"cmd/compile/internal/p"}, Units: []Unit{u}}
	dir := t.TempDir()
	writeTestFile(t, dir, "internal/p/adapter_context.go", "package p;type Context struct{count int}")
	r, err := Render(s, plan, dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := Install(dir, r); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, dir, "go.mod", "module go-backend-ssa\n\ngo 1.27.1\n")
	writeTestFile(t, dir, "internal/p/check_test.go", `package p
 import "testing"
 func TestGeneric(t *testing.T){a,b:=newContext(),newContext();if !one(int8(2))||one(int8(-128))!=true||!pair(int8(4),int64(8))||pair(int8(3),int64(8)){t.Fatal("generic semantics")};if !a.Apply(16)||a.Apply(3)||!b.Apply(2)||a.count!=2||b.count!=1{t.Fatal("generic function value or compilation state")}}
 `)
	cmd := exec.Command("go", "test", "-race", "./...")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOWORK=off", "GOTOOLCHAIN=go1.27.1")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	for name, changed := range map[string]string{
		"direct state":     strings.Replace(source, "return v!=0", "count++;return v!=0", 1),
		"transitive state": strings.Replace(source, "return v!=0", "tracked();return v!=0", 1),
	} {
		t.Run(name, func(t *testing.T) {
			s.cache[u.Source] = []byte(changed)
			if _, err := Render(s, plan, dir); err == nil || !strings.Contains(err.Error(), "stateless function uses compilation state") {
				t.Fatalf("generic helper gained state: %v", err)
			}
		})
	}
}

package port

import (
	"context"
	"strings"
	"testing"
)

func TestInspectClosureStatesAndVariants(t *testing.T) {
	repo, pin := fixtureRepo(t, map[string]string{
		"src/cmd/compile/internal/a/a.go": `package a
import ("internal/b"; "os")
var Counter int
var Table=map[int]int{}
var Callback=func() int {return Counter}
func F() { Counter++; Table[1]=Counter; b.Value=Counter; os.Getenv("X"); _=Callback }
func Shadow(Counter int) int { return Counter }
`,
		"src/cmd/compile/internal/a/a_windows.go": "//go:build windows\n\npackage a\nvar WindowsOnly int\n",
		"src/cmd/compile/internal/a/gen.go":       "//go:build ignore\n\npackage main\nvar ToolOnly int\n",
		"src/cmd/compile/internal/a/a_test.go":    "package a\nvar TestOnly int\n",
		"src/internal/b/b.go":                     "package b\nvar Value int\n",
	})
	s, e := OpenSource(context.Background(), repo, pin)
	if e != nil {
		t.Fatal(e)
	}
	r, e := Inspect(s, []string{"cmd/compile/internal/a"})
	if e != nil {
		t.Fatal(e)
	}
	if len(r.Packages) != 2 {
		t.Fatalf("closure: %d", len(r.Packages))
	}
	pr := r.Packages[0]
	vars := map[string]bool{}
	for _, d := range pr.Declarations {
		if d.Kind == "var" {
			vars[d.ID] = true
			if d.StateReview == "" {
				t.Fatal("unclassified variable omitted")
			}
		}
	}
	for _, name := range []string{"Counter", "Table", "Callback", "WindowsOnly"} {
		if !vars[pr.Path+"."+name] {
			t.Fatalf("missing %s", name)
		}
	}
	if len(vars) != 4 {
		t.Fatalf("included tool/test variables: %v", vars)
	}
	for _, d := range pr.Declarations {
		if strings.HasSuffix(d.ID, ".Callback") && !strings.Contains(strings.Join(d.InitializerDependencies, ","), pr.Path+".Counter") {
			t.Fatal("initializer dependency omitted")
		}
	}
	uses := map[string]bool{}
	for _, r := range pr.References {
		if strings.HasSuffix(r.From, ".Shadow") && strings.HasSuffix(r.To, ".Counter") {
			t.Fatal("local shadow misclassified")
		}
		uses[r.To+":"+r.Use] = true
	}
	for _, key := range []string{pr.Path + ".Counter:read-write", pr.Path + ".Table:write-through", "internal/b.Value:write-through"} {
		if !uses[key] {
			t.Fatalf("missing reference %s: %v", key, uses)
		}
	}
	if len(pr.Hazards) == 0 {
		t.Fatal("environment read not recorded")
	}
	b1, _ := JSON(r)
	r2, e := Inspect(s, []string{"cmd/compile/internal/a"})
	if e != nil {
		t.Fatal(e)
	}
	b2, _ := JSON(r2)
	if string(b1) != string(b2) {
		t.Fatal("nondeterministic inventory")
	}
}

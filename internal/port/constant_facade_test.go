package port

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestPublicConstantFacades(t *testing.T) {
	source := `package p
 type Kind uint8
 const(Zero Kind=iota;_;Second;hidden;Fourth)
 const(Text="hello";Huge=1<<80;Typed Kind=9;Flag=true;Alias=Typed)
 `
	repo, pin := fixtureRepo(t, map[string]string{"LICENSE": "fixture", "PATENTS": "fixture", "src/cmd/compile/internal/p/p.go": source})
	s, err := OpenSource(context.Background(), repo, pin)
	if err != nil {
		t.Fatal(err)
	}
	unit := Unit{Source: "src/cmd/compile/internal/p/p.go", Declarations: []string{"Kind", "Zero", "Text"}, ExportConstants: true}
	plan := Plan{InspectRoots: []string{"cmd/compile/internal/p"}, Units: []Unit{unit}}
	dir := t.TempDir()
	r, err := Render(s, plan, dir)
	if err != nil {
		t.Fatal(err)
	}
	output := unit.constantsOutput()
	facade := string(r.Files[output])
	if strings.Contains(facade, "hidden") || !strings.Contains(facade, "compiler.Second") || !strings.Contains(facade, "compiler.Huge") {
		t.Fatal("facade does not preserve exported constant aliases", facade)
	}
	if err := Install(dir, r); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, dir, "go.mod", "module go-backend-ssa\n\ngo 1.27.1\n")
	api := `package p;import compiler "go-backend-ssa/internal/p";type Kind=compiler.Kind`
	writeTestFile(t, dir, "p/api.go", api)
	consumer := t.TempDir()
	writeTestFile(t, consumer, "go.mod", fmt.Sprintf("module consumer\n\ngo 1.27.1\nrequire go-backend-ssa v0.0.0\nreplace go-backend-ssa => %q\n", dir))
	writeTestFile(t, consumer, "consumer_test.go", `package consumer
 import("testing";"go-backend-ssa/p")
 const big=p.Huge>>80
 var nativeKind p.Kind=p.Second
 func TestAPI(t *testing.T){if nativeKind!=2||p.Fourth!=4||p.Typed!=9||p.Alias!=p.Typed||big!=1||p.Text!="hello"||!p.Flag{t.Fatal("native constant identity or untyped semantics changed")}}
 `)
	cmd := exec.Command("go", "test", "-race", "./...")
	cmd.Dir = consumer
	cmd.Env = append(os.Environ(), "GOWORK=off", "GOTOOLCHAIN=go1.27.1")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("consumer: %v\n%s", err, out)
	}
	// Public generated files use the same ownership and stale-file rules as the
	// ported native source; handwritten API files are outside that ownership.
	reduced := plan
	reduced.Units = append([]Unit(nil), plan.Units...)
	reduced.Units[0].ExportConstants = false
	without, err := Render(s, reduced, dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := Install(dir, without); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, output)); !os.IsNotExist(err) {
		t.Fatal("stale facade retained", err)
	}
	if data, err := os.ReadFile(filepath.Join(dir, "p/api.go")); err != nil || string(data) != api {
		t.Fatal("handwritten public API changed", err)
	}
	if err := Install(dir, r); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, dir, output, facade+"\n// user edit\n")
	if err := Install(dir, r); err == nil || !strings.Contains(err.Error(), "edited file") {
		t.Fatal("edited facade overwritten", err)
	}
	for _, source := range []string{"src/cmd/compile/internal/p/nested/p.go", "src/cmd/internal/obj/p.go"} {
		bad := plan
		bad.Units = append([]Unit(nil), plan.Units...)
		bad.Units[0].Source = source
		if err := bad.Validate(); err == nil {
			t.Fatal("unsupported facade path accepted", source)
		}
	}
}

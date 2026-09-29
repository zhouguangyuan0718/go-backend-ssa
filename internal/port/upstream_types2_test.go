package port

import (
	"bytes"
	"context"
	"go/ast"
	"go/format"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// Exercise the original types2 basic-type tables, whose nested, positional
// pointer literals must gain per-instance ownership before the full checker can
// be ported. This is a migration test fixture, not a partial public checker API.
func testUpstreamTypes2Tables(t *testing.T, s *Source) {
	t.Helper()
	plan := Plan{InspectRoots: []string{"cmd/compile/internal/types2"}, Units: []Unit{
		{Source: "src/cmd/compile/internal/types2/basic.go", Declarations: []string{"BasicKind", "Invalid", "BasicInfo", "IsBoolean", "Basic", "Basic.Kind", "Basic.Info", "Basic.Name"}, Owner: &OwnerRule{Types: []string{"Basic"}, Context: "tableContext", Field: "owner"}},
		{Source: "src/cmd/compile/internal/types2/universe.go", Context: &ContextRule{Type: "tableContext", Constructor: "newTableContext", Adapter: "adapter_context.go", Fields: map[string]string{"Typ": "Typ", "basicAliases": "basicAliases"}}},
	}}
	// Isolate the reviewed table declarations as a fixture: the production
	// selector correctly refuses to drop universe.go's unrelated scope init.
	blobs := map[string]string{"LICENSE": "fixture", "PATENTS": "fixture"}
	rawFiles := map[string]string{}
	for _, u := range plan.Units {
		data, err := s.Read(u.Source)
		if err != nil {
			t.Fatal(err)
		}
		selected := u
		if u.Context != nil {
			c := *u.Context
			c.Init = true
			selected.Context = &c
		}
		fs := token.NewFileSet()
		f, err := selectFile(fs, u.Source, data, selected)
		if err != nil {
			t.Fatal(err)
		}
		var decls []ast.Decl
		for _, d := range f.Decls {
			if fn, ok := d.(*ast.FuncDecl); ok && fn.Recv == nil && fn.Name.Name == "init" {
				continue
			}
			decls = append(decls, d)
		}
		f.Decls = decls
		pruneImports(f)
		var source bytes.Buffer
		if err := format.Node(&source, fs, f); err != nil {
			t.Fatal(err)
		}
		blobs[u.Source] = source.String()
		rawFiles[filepath.Base(u.Source)] = source.String()
	}
	repo, pin := fixtureRepo(t, blobs)
	fixture, err := OpenSource(context.Background(), repo, pin)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	writeTestFile(t, dir, "internal/types2/adapter_context.go", `package types2;type tableContext struct{Typ [26]*Basic;basicAliases [2]*Basic}`)
	rendered, err := Render(fixture, plan, dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := Install(dir, rendered); err != nil {
		t.Fatal(err)
	}
	for name, raw := range rawFiles {
		writeTestFile(t, dir, "internal/reference/"+name, raw)
	}
	writeTestFile(t, dir, "go.mod", "module go-backend-ssa\n\ngo 1.27.1\n")
	writeTestFile(t, dir, "internal/reference/adapter_snapshot.go", `package types2
import "fmt"
func Snapshot()[]string{var out []string;for i,b:=range Typ{out=append(out,fmt.Sprintf("%d:%d:%d:%s",i,b.Kind(),b.Info(),b.Name()))};for _,b:=range basicAliases{out=append(out,fmt.Sprintf("alias:%d:%d:%s",b.Kind(),b.Info(),b.Name()))};return out}
`)
	writeTestFile(t, dir, "internal/types2/adapter_test.go", `package types2
import("fmt";"reflect";"sync";"testing";native "go-backend-ssa/internal/reference")
func TestTables(t *testing.T){want:=native.Snapshot();a,b:=newTableContext(),newTableContext();var out []string;for i,x:=range a.Typ{if x.owner!=a||x==b.Typ[i]{t.Fatal("shared basic node")};out=append(out,fmt.Sprintf("%d:%d:%d:%s",i,x.Kind(),x.Info(),x.Name()))};for i,x:=range a.basicAliases{if x.owner!=a||x==a.Typ[x.Kind()]||x==b.basicAliases[i]{t.Fatal("alias identity")};out=append(out,fmt.Sprintf("alias:%d:%d:%s",x.Kind(),x.Info(),x.Name()))};if !reflect.DeepEqual(out,want){t.Fatal(out,want)};saved:=a.Typ[Int];b.Typ[Int].name="changed";var wg sync.WaitGroup;for range 24{wg.Go(func(){c:=newTableContext();if c.Typ[Int].Name()!="int"||c.Typ[Int].owner!=c{t.Error("concurrent initializer")}})};wg.Wait();if saved!=a.Typ[Int]||saved.Name()!="int"{t.Fatal("old graph changed")}}
`)
	cmd := exec.Command("go", "test", "-race", "./internal/types2")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOWORK=off", "GOTOOLCHAIN=go1.27.1")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("types2 table differential at %s: %v\n%s", s.Pin.Tag, err, out)
	}
	t.Logf("types2 table initialization/ownership at %s:\n%s", s.Pin.Tag, out)
}

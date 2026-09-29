package port

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestExternalStateIsolationAndSignatureDrift(t *testing.T) {
	const host = `package host
 type Flags struct { SIMD bool }
 var Current Flags = environmentThatMustNotBeCopied()
 var Count int
 `
	const source = `package p
 import h "cmd/compile/internal/host"
 type Node struct{}
 func Set(n int) { h.Count=n; h.Current.SIMD=n>0 }
 func Pointer()*int { return &h.Count }
 func Callback()func()int {return func()int{return h.Count}}
 func(n *Node) Bump(){h.Count++}
 func Shadow()int {h:=struct{Count int}{7};return h.Count}
 `
	repo, pin := fixtureRepo(t, map[string]string{"LICENSE": "fixture", "PATENTS": "fixture", "src/cmd/compile/internal/host/host.go": host, "src/cmd/compile/internal/p/p.go": source})
	s, err := OpenSource(context.Background(), repo, pin)
	if err != nil {
		t.Fatal(err)
	}
	stateSource := "src/cmd/compile/internal/host/host.go"
	plan := Plan{InspectRoots: []string{"cmd/compile/internal/p"}, Units: []Unit{
		{Source: stateSource, Declarations: []string{"Flags"}},
		{Source: "src/cmd/compile/internal/p/p.go", Functions: []string{"Set", "Pointer", "Callback", "Shadow"}, Declarations: []string{"Node", "Node.Bump"}, Owner: &OwnerRule{Types: []string{"Node"}, Context: "Context", Field: "owner"}, Context: &ContextRule{Type: "Context", Constructor: "newContext", Adapter: "adapter_context.go"}, State: []StateRule{{Source: stateSource, Variable: "Current", Field: "flags"}, {Source: stateSource, Variable: "Count", Field: "count"}}},
	}}
	dir := t.TempDir()
	const adapter = `package p
 import "go-backend-ssa/internal/host"
 type Context struct{flags host.Flags;count int}
 `
	writeTestFile(t, dir, "internal/p/adapter_context.go", adapter)
	r, err := Render(s, plan, dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, prov := range r.Ledger.Files {
		if prov.Source == plan.Units[1].Source && prov.SignatureSources[stateSource] != hash([]byte(host)) {
			t.Fatal("state source hash missing")
		}
	}
	if strings.Contains(string(r.Files["internal/host/host.go"]), "environmentThatMustNotBeCopied") {
		t.Fatal("copied host initializer")
	}
	if err := Install(dir, r); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, dir, "go.mod", "module go-backend-ssa\n\ngo 1.27.1\n")
	writeTestFile(t, dir, "internal/p/adapter_test.go", `package p
 import("testing";"sync")
 func TestState(t *testing.T){a,b:=newContext(),newContext();a.Set(1);b.Set(2);old:=a.Callback();ptr:=a.Pointer();*ptr=3;(&Node{owner:b}).Bump();if old()!=3||b.count!=3||!a.flags.SIMD||a.Shadow()!=7{t.Fatal("state isolation")};b.Set(0);if !a.flags.SIMD||b.flags.SIMD{t.Fatal("nested state")};var wg sync.WaitGroup;for i:=range 16{wg.Go(func(){c:=newContext();c.Set(i);for range 100{(&Node{owner:c}).Bump()};if c.Callback()()!=i+100{t.Error("parallel state")}})};wg.Wait()}
 `)
	cmd := exec.Command("go", "test", "-race", "./...")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOWORK=off", "GOTOOLCHAIN=go1.27.1")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	for _, tc := range []struct{ name, source, want string }{
		{"type drift", strings.Replace(host, "Count int", "Count int64", 1), "selected package"},
		{"inferred variable", strings.Replace(host, "var Count int", "var Count = 0", 1), "explicit type"},
		{"missing variable", strings.Replace(host, "var Count int", "var Renamed int", 1), "not found"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s.cache[stateSource] = []byte(tc.source)
			defer func() { s.cache[stateSource] = []byte(host) }()
			if _, err := Render(s, plan, dir); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("want %s, got %v", tc.want, err)
			}
		})
	}
	// Even assignment-compatible types are checked against the pinned signature.
	writeTestFile(t, dir, "internal/p/adapter_context.go", strings.Replace(adapter, "count int", "count any", 1))
	if _, err := Render(s, plan, dir); err == nil {
		t.Fatal("wrong state adapter accepted")
	}
}

func TestImportedZeroValues(t *testing.T) {
	const host = `package host; type Pos struct{N int};var Missing Pos`
	const source = `package p
 import alias "cmd/compile/internal/host"
 func Absent()*alias.Pos{return &alias.Missing}
 func Zero(p alias.Pos)bool{if p==alias.Missing{return true};return false}
 func Shadow()int{alias:=struct{Missing int}{9};return alias.Missing}
 `
	repo, pin := fixtureRepo(t, map[string]string{"LICENSE": "fixture", "PATENTS": "fixture", "src/cmd/compile/internal/host/host.go": host, "src/cmd/compile/internal/p/p.go": source})
	s, err := OpenSource(context.Background(), repo, pin)
	if err != nil {
		t.Fatal(err)
	}
	plan := Plan{InspectRoots: []string{"cmd/compile/internal/p"}, Units: []Unit{{Source: "src/cmd/compile/internal/host/host.go", WholeFile: true, ZeroValues: []string{"Missing"}}, {Source: "src/cmd/compile/internal/p/p.go", WholeFile: true}}}
	dir := t.TempDir()
	r, err := Render(s, plan, dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := Install(dir, r); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, dir, "go.mod", "module go-backend-ssa\n\ngo 1.27.1\n")
	writeTestFile(t, dir, "internal/p/adapter_test.go", `package p;import "testing";func TestZero(t *testing.T){a,b:=Absent(),Absent();a.N=1;if b.N!=0||!Zero(*b)||Zero(*a)||Shadow()!=9{t.Fatal("imported zero shared or shadow rewritten")}}`)
	cmd := exec.Command("go", "test", "-race", "./...")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOWORK=off", "GOTOOLCHAIN=go1.27.1")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	s.cache[plan.Units[1].Source] = []byte(source + "\nfunc Reset(){alias.Missing=alias.Pos{}}")
	if _, err := Render(s, plan, dir); err == nil || !strings.Contains(err.Error(), "transformed package") {
		t.Fatalf("imported sentinel write accepted: %v", err)
	}
}

func TestHostCallResultTypes(t *testing.T) {
	const host = `package host
 import "os"
 var Root=os.Getenv("GOROOT")
 var Target=readTarget()
 func readTarget()string{panic("must not be copied")}
 `
	const source = `package p
 import h "cmd/compile/internal/host"
 func Inputs()string{return h.Root+":"+h.Target}
 `
	const adapter = `package p
 type Context struct{Root,Target string}
 `
	repo, pin := fixtureRepo(t, map[string]string{"LICENSE": "fixture", "PATENTS": "fixture", "src/cmd/compile/internal/host/host.go": host, "src/cmd/compile/internal/p/p.go": source})
	s, err := OpenSource(context.Background(), repo, pin)
	if err != nil {
		t.Fatal(err)
	}
	plan := Plan{InspectRoots: []string{"cmd/compile/internal/p"}, Units: []Unit{{Source: "src/cmd/compile/internal/p/p.go", WholeFile: true, Context: &ContextRule{Type: "Context", Constructor: "NewContext", Adapter: "adapter_context.go"}, State: []StateRule{{Source: "src/cmd/compile/internal/host/host.go", Variable: "Root", Field: "Root"}, {Source: "src/cmd/compile/internal/host/host.go", Variable: "Target", Field: "Target"}}}}}
	dir := t.TempDir()
	writeTestFile(t, dir, "internal/p/adapter_context.go", adapter)
	r, err := Render(s, plan, dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, data := range r.Files {
		if strings.Contains(string(data), "Getenv") || strings.Contains(string(data), "readTarget") {
			t.Fatal("host initializer copied")
		}
	}
	if err := Install(dir, r); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, dir, "go.mod", "module go-backend-ssa\n\ngo 1.27.1\n")
	writeTestFile(t, dir, "internal/p/context_test.go", `package p
 import "testing"
 func TestInputs(t *testing.T){a,b:=NewContext(),NewContext();a.Root="/sdk";a.Target="arm64";b.Target="386";if a.Inputs()!="/sdk:arm64"||b.Inputs()!=":386"{t.Fatal("shared or implicit host inputs")}}
 `)
	cmd := exec.Command("go", "test", "./...")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOWORK=off", "GOTOOLCHAIN=go1.27.1")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	s.cache["src/cmd/compile/internal/host/host.go"] = []byte(strings.Replace(host, "func readTarget()string", "func readTarget()int", 1))
	if _, err := Render(s, plan, dir); err == nil || !strings.Contains(err.Error(), "mismatch") {
		t.Fatalf("host result drift accepted: %v", err)
	}
	s.cache["src/cmd/compile/internal/host/host.go"] = []byte(strings.Replace(host, "func readTarget()string", "func readTarget()(string,error)", 1))
	if _, err := Render(s, plan, dir); err == nil || !strings.Contains(err.Error(), "single-result") {
		t.Fatalf("multiple results accepted: %v", err)
	}
}

package port

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestExternalNamedCompositeState(t *testing.T) {
	const hostPath = "src/cmd/compile/internal/host/host.go"
	const host = `package host
 type Descriptor struct{Size int;Hook func()}
 var Target=Descriptor{Size:readHostSize(),Hook:unportedHostHook}
 `
	const body = `package p;import "cmd/compile/internal/host";func Address()*host.Descriptor{return &host.Target};func Size()int{return host.Target.Size}`
	repo, pin := fixtureRepo(t, map[string]string{"LICENSE": "fixture", "PATENTS": "fixture", hostPath: host, "src/cmd/compile/internal/p/p.go": body})
	s, err := OpenSource(context.Background(), repo, pin)
	if err != nil {
		t.Fatal(err)
	}
	plan := Plan{InspectRoots: []string{"cmd/compile/internal/p"}, Units: []Unit{
		{Source: hostPath, Declarations: []string{"Descriptor"}},
		{Source: "src/cmd/compile/internal/p/p.go", WholeFile: true, Context: &ContextRule{Type: "Context", Constructor: "newContext", Adapter: "adapter_context.go", Selective: true}, State: []StateRule{{Source: hostPath, Variable: "Target", Field: "Target"}}},
	}}
	dir := t.TempDir()
	writeTestFile(t, dir, "internal/p/adapter_context.go", `package p;import "go-backend-ssa/internal/host";type Context struct{Target host.Descriptor}`)
	r, err := Render(s, plan, dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, data := range r.Files {
		if strings.Contains(string(data), "readHostSize") || strings.Contains(string(data), "unportedHostHook") {
			t.Fatal("host initializer entered output")
		}
	}
	if err := Install(dir, r); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, dir, "go.mod", "module go-backend-ssa\n\ngo 1.27.1\n")
	writeTestFile(t, dir, "internal/p/adapter_test.go", `package p;import "testing";func TestValues(t *testing.T){a,b:=newContext(),newContext();a.Target.Size=8;b.Target.Size=4;if a.Address()!=&a.Target||b.Address()!=&b.Target||a.Size()!=8||b.Size()!=4{t.Fatal("descriptor identity or state leaked")}}`)
	cmd := exec.Command("go", "test", "./...")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOWORK=off", "GOTOOLCHAIN=go1.27.1")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	s.cache[hostPath] = []byte(strings.Replace(host, "var Target=Descriptor{Size:readHostSize(),Hook:unportedHostHook}", "var Target=[]int{1,2}", 1))
	if _, err := Render(s, plan, dir); err == nil || !strings.Contains(err.Error(), "named type") {
		t.Fatalf("unnamed composite accepted: %v", err)
	}
}

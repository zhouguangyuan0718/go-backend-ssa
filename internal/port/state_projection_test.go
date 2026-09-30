package port

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestProjectedExternalState(t *testing.T) {
	host := `package host
 type Link struct { Name string }
 type ArchInfo struct { LinkArch *Link; SoftFloat bool; Backend func(*Unavailable) }
 var Arch ArchInfo
 `
	source := `package p
 import h "cmd/compile/internal/host"
 var initial=h.Arch.SoftFloat
 func Initial()bool{return initial}
 func Enabled()bool{return h.Arch.SoftFloat}
 func Name()string{return (h.Arch).LinkArch.Name}
 func Ref()*bool{return &h.Arch.SoftFloat}
 func Callback()func()bool{return func()bool{return h.Arch.SoftFloat}}
 func Shadow()bool{h:=struct{Arch struct{SoftFloat bool}}{};return h.Arch.SoftFloat}
 `
	repo, pin := fixtureRepo(t, map[string]string{"LICENSE": "fixture", "PATENTS": "fixture", "src/cmd/compile/internal/host/arch.go": host, "src/cmd/compile/internal/p/p.go": source})
	s, err := OpenSource(context.Background(), repo, pin)
	if err != nil {
		t.Fatal(err)
	}
	plan := Plan{InspectRoots: []string{"cmd/compile/internal/p"}, Units: []Unit{
		{Source: "src/cmd/compile/internal/host/arch.go", Declarations: []string{"Link"}},
		{Source: "src/cmd/compile/internal/p/p.go", WholeFile: true, Context: &ContextRule{Type: "Context", Constructor: "initialize", Initialize: true, Fields: map[string]string{"initial": "initial"}, Selective: true, Adapter: "adapter_context.go"}, State: []StateRule{
			{Source: "src/cmd/compile/internal/host/arch.go", Variable: "Arch.LinkArch", Field: "Link"},
			{Source: "src/cmd/compile/internal/host/arch.go", Variable: "Arch.SoftFloat", Field: "SoftFloat"},
		}}}}
	dir := t.TempDir()
	adapter := `package p;import "go-backend-ssa/internal/host";type Context struct {Link *host.Link;SoftFloat bool;initial bool};func NewContext(link *host.Link,soft bool)*Context{c:=&Context{Link:link,SoftFloat:soft};c.initialize();return c}`
	writeTestFile(t, dir, "internal/p/adapter_context.go", adapter)
	r, err := Render(s, plan, dir)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(r.Files["internal/p/p.go"]), "h.Arch") != 1 {
		t.Fatal("projected global or shadowed local was rewritten incorrectly", string(r.Files["internal/p/p.go"]))
	}
	if err := Install(dir, r); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, dir, "go.mod", "module go-backend-ssa\n\ngo 1.27.1\n")
	writeTestFile(t, dir, "internal/p/adapter_test.go", `package p
 import("fmt";"testing";"go-backend-ssa/internal/host")
 func TestProjection(t *testing.T){old:=NewContext(&host.Link{Name:"old"},true);cb:=old.Callback();for i:=0;i<8;i++{t.Run(fmt.Sprint(i),func(t *testing.T){t.Parallel();c:=NewContext(&host.Link{Name:"new"},false);if c.Initial()||!old.Initial()||c.Enabled()||!cb()||old.Name()!="old"||c.Name()!="new"||Shadow(){t.Fatal("instance state leaked")};*c.Ref()=true;if !c.Enabled(){t.Fatal("field address lost")}})}}
 `)
	cmd := exec.Command("go", "test", "-race", "./...")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOWORK=off", "GOTOOLCHAIN=go1.27.1")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	for _, body := range []string{"func Escape()any{return h.Arch}", "func Address()any{return &h.Arch}", "func Unreviewed()any{return h.Arch.Backend}"} {
		s.cache[plan.Units[1].Source] = []byte(source + body)
		if _, err := Render(s, plan, dir); err == nil {
			t.Fatal("uncovered projected state use accepted", body)
		}
	}
	s.cache[plan.Units[1].Source] = []byte(strings.Replace(source, `import h "cmd/compile/internal/host"`, `import (h "cmd/compile/internal/host";"unsafe")`, 1) + `func Offset()uintptr{return unsafe.Offsetof(h.Arch.SoftFloat)}`)
	if _, err := Render(s, plan, dir); err == nil || !strings.Contains(err.Error(), "Offsetof") {
		t.Fatalf("projected layout observation accepted: %v", err)
	}
	s.cache[plan.Units[1].Source] = []byte(source)
	s.cache[plan.Units[0].Source] = []byte(strings.Replace(host, "SoftFloat bool", "Renamed bool", 1))
	if _, err := Render(s, plan, dir); err == nil {
		t.Fatal("upstream field drift accepted")
	}
	s.cache[plan.Units[0].Source] = []byte(host)
	writeTestFile(t, dir, "internal/p/adapter_context.go", strings.Replace(adapter, "SoftFloat bool", "SoftFloat string", 1))
	if _, err := Render(s, plan, dir); err == nil {
		t.Fatal("wrong adapter field type accepted")
	}
}

func TestProjectedStateAcrossConsumers(t *testing.T) {
	host := `package host;type ArchInfo struct{Soft bool;Width int;Unused func(*Unavailable)};var Arch ArchInfo`
	repo, pin := fixtureRepo(t, map[string]string{"LICENSE": "fixture", "PATENTS": "fixture", "src/cmd/compile/internal/host/arch.go": host, "src/cmd/compile/internal/p/p.go": `package p;import "cmd/compile/internal/host";func Get()bool{return host.Arch.Soft}`, "src/cmd/compile/internal/q/q.go": `package q;import "cmd/compile/internal/host";func Get()int{return host.Arch.Width}`})
	source, err := OpenSource(context.Background(), repo, pin)
	if err != nil {
		t.Fatal(err)
	}
	plan := Plan{InspectRoots: []string{"cmd/compile/internal/p", "cmd/compile/internal/q"}}
	dir := t.TempDir()
	for _, row := range []struct{ pkg, field, typ string }{{"p", "Soft", "bool"}, {"q", "Width", "int"}} {
		plan.Units = append(plan.Units, Unit{Source: "src/cmd/compile/internal/" + row.pkg + "/" + row.pkg + ".go", WholeFile: true, Context: &ContextRule{Type: "Context", Constructor: "newContext", Selective: true, Adapter: "adapter_context.go"}, State: []StateRule{{Source: "src/cmd/compile/internal/host/arch.go", Variable: "Arch." + row.field, Field: row.field}}})
		writeTestFile(t, dir, "internal/"+row.pkg+"/adapter_context.go", "package "+row.pkg+";type Context struct{"+row.field+" "+row.typ+"}")
	}
	r, err := Render(source, plan, dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range r.Ledger.Files {
		if strings.HasSuffix(p.Output, ".go") && p.SignatureSources["src/cmd/compile/internal/host/arch.go"] != hash([]byte(host)) {
			t.Fatal("full signature source not recorded", p.Output)
		}
	}
	plan.Units[1].State = append(plan.Units[1].State, StateRule{Source: "src/cmd/compile/internal/host/arch.go", Variable: "Arch", Field: "Whole"})
	if _, err := Render(source, plan, dir); err == nil || !strings.Contains(err.Error(), "whole and projected") {
		t.Fatalf("ambiguous full-value rule: %v", err)
	}
}

package port

import (
	"context"
	"strings"
	"testing"
)

func TestNativeFunctionRegionAssignments(t *testing.T) {
	const file = "src/cmd/compile/internal/p/p.go"
	const source = `package p
var state struct{A,B,C int}
func Defaults(host func()){host();state.A=3;state.B=7;state.C=9;host()}
`
	repo, pin := fixtureRepo(t, map[string]string{"LICENSE": "fixture", "PATENTS": "fixture", file: source})
	s, e := OpenSource(context.Background(), repo, pin)
	if e != nil {
		t.Fatal(e)
	}
	u := Unit{Source: file, Functions: []string{"Defaults"}, Context: &ContextRule{Type: "Context", Constructor: "newContext", Selective: true, Adapter: "adapter_context.go", Fields: map[string]string{"state": "state"}}, FunctionRegions: map[string]FunctionRegionRule{"Defaults": {First: SourceCallAnchor{Assignment: "state.A"}, Last: SourceCallAnchor{Assignment: "state.B"}}}}
	plan := Plan{InspectRoots: []string{"cmd/compile/internal/p"}, Units: []Unit{u}}
	dir := t.TempDir()
	writeTestFile(t, dir, "internal/p/adapter_context.go", `package p;type Context struct{state struct{A,B,C int}}`)
	r, e := Render(s, plan, dir)
	if e != nil {
		t.Fatal(e)
	}
	out := string(r.Files["internal/p/p.go"])
	if strings.Contains(out, "host()") || strings.Contains(out, "state.C = 9") || !strings.Contains(out, "syncgoCtx.state.B = 7") {
		t.Fatal(out)
	}
	for name, source := range map[string]string{"shadow": strings.Replace(source, "host();state.A", "host();state:=struct{A,B,C int}{};state.A", 1), "compound": strings.Replace(source, "state.A=3", "state.A+=3", 1), "duplicate": strings.Replace(source, "state.C=9", "state.A=9", 1)} {
		t.Run(name, func(t *testing.T) {
			s.cache[file] = []byte(source)
			if _, e := Render(s, plan, dir); e == nil {
				t.Fatal("invalid assignment accepted")
			}
		})
	}
}

func TestFunctionRegionCopiesPreserveGlobalSourceIdentity(t *testing.T) {
	const p = "src/cmd/compile/internal/p/p.go"
	const q = "src/cmd/compile/internal/q/q.go"
	repo, pin := fixtureRepo(t, map[string]string{"LICENSE": "fixture", "PATENTS": "fixture", p: `package p
var Flag struct{A,B int}
func Main(){Flag.A=1;Flag.B=2}
`, q: `package q;import "cmd/compile/internal/p";func Read()int{return p.Flag.A}`})
	source, e := OpenSource(context.Background(), repo, pin)
	if e != nil {
		t.Fatal(e)
	}
	region := FunctionRegionRule{First: SourceCallAnchor{Assignment: "Flag.A"}, Last: SourceCallAnchor{Assignment: "Flag.B"}}
	copied := region
	copied.SourceFunction = "Main"
	plan := Plan{InspectRoots: []string{"cmd/compile/internal/p", "cmd/compile/internal/q"}, Units: []Unit{
		{Source: p, Functions: []string{"Main", "Copy"}, Context: &ContextRule{Type: "Context", Constructor: "newContext", Adapter: "adapter_context.go", Selective: true, Fields: map[string]string{"Flag": "Flag"}}, FunctionRegions: map[string]FunctionRegionRule{"Main": region, "Copy": copied}},
		{Source: q, Functions: []string{"Read"}, Context: &ContextRule{Type: "Context", Constructor: "newContext", Adapter: "adapter_context.go", Selective: true}, State: []StateRule{{Source: p, Variable: "Flag", Field: "Flags"}}},
	}}
	root := t.TempDir()
	writeTestFile(t, root, "internal/p/adapter_context.go", `package p;type Context struct{Flag struct{A,B int}}`)
	writeTestFile(t, root, "internal/q/adapter_context.go", `package q;type Context struct{Flags struct{A,B int}}`)
	result, e := Render(source, plan, root)
	if e != nil {
		t.Fatal(e)
	}
	if !strings.Contains(string(result.Files["internal/q/q.go"]), "syncgoCtx.Flags.A") {
		t.Fatal("external state binding missing")
	}
}

func TestEnclosingAssignmentRegion(t *testing.T) {
	const file = "src/cmd/compile/internal/p/p.go"
	const source = `package p
var Flag struct{N,End int}
func Main(){Flag.End++;if Flag.N!=0{Flag.N=0;Flag.End++};Flag.End=7;Flag.End++}
`
	repo, pin := fixtureRepo(t, map[string]string{"LICENSE": "fixture", "PATENTS": "fixture", file: source})
	input, e := OpenSource(context.Background(), repo, pin)
	if e != nil {
		t.Fatal(e)
	}
	unit := Unit{Source: file, Functions: []string{"Main"}, Context: &ContextRule{Type: "Context", Constructor: "newContext", Adapter: "adapter_context.go", Selective: true, Fields: map[string]string{"Flag": "Flag"}}, FunctionRegions: map[string]FunctionRegionRule{"Main": {First: SourceCallAnchor{Assignment: "Flag.N", Enclosing: "if"}, Last: SourceCallAnchor{Assignment: "Flag.End"}}}}
	plan := Plan{InspectRoots: []string{"cmd/compile/internal/p"}, Units: []Unit{unit}}
	root := t.TempDir()
	writeTestFile(t, root, "internal/p/adapter_context.go", `package p;type Context struct{Flag struct{N,End int}}`)
	result, e := Render(input, plan, root)
	if e != nil {
		t.Fatal(e)
	}
	text := string(result.Files["internal/p/p.go"])
	if strings.Count(text, "syncgoCtx.Flag.End++") != 1 || !strings.Contains(text, "if syncgoCtx.Flag.N != 0") || !strings.Contains(text, "syncgoCtx.Flag.End = 7") {
		t.Fatal(text)
	}
	for name, bad := range map[string]string{
		"duplicate": strings.Replace(source, "Flag.N=0", "Flag.N=0;Flag.N=1", 1),
		"shadow":    strings.Replace(source, "Flag.N=0", "Flag:=struct{N int}{};Flag.N=0", 1),
		"closure":   strings.Replace(source, "Flag.N=0", "func(){Flag.N=0}()", 1),
	} {
		t.Run(name, func(t *testing.T) {
			input.cache[file] = []byte(bad)
			if _, e := Render(input, plan, root); e == nil {
				t.Fatal("invalid enclosing assignment accepted")
			}
		})
	}
}

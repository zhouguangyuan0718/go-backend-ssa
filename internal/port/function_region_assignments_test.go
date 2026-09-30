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

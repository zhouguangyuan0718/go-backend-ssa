package port

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestNativeHostStreamField(t *testing.T) {
	const path = "src/cmd/compile/internal/p/p.go"
	const source = `package p;import "os";type Output struct{file *os.File};func(o *Output)Move(n int64)(int64,error){return o.file.Seek(n,1)}`
	repo, pin := fixtureRepo(t, map[string]string{"LICENSE": "fixture", "PATENTS": "fixture", path: source})
	s, err := OpenSource(context.Background(), repo, pin)
	if err != nil {
		t.Fatal(err)
	}
	plan := Plan{InspectRoots: []string{"cmd/compile/internal/p"}, Units: []Unit{{Source: path, WholeFile: true, HostFields: []HostFieldRule{{Type: "Output", Field: "file", Methods: []string{"Seek"}}}, Context: &ContextRule{Type: "Context", Constructor: "initialize", Initialize: true, Selective: true, Adapter: "adapter_context.go"}}}}
	dir := t.TempDir()
	writeTestFile(t, dir, "internal/p/adapter_context.go", `package p;import "io";type Context struct{};func NewOutput(s io.Seeker)*Output{return &Output{file:s}}`)
	r, err := Render(s, plan, dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := Install(dir, r); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, dir, "go.mod", "module go-backend-ssa\n\ngo 1.27.1\n")
	writeTestFile(t, dir, "internal/p/check_test.go", `package p;import("errors";"testing");type stream struct{off int64;failure error};func(s *stream)Seek(n int64,w int)(int64,error){if w!=1{panic(w)};s.off+=n;return s.off,s.failure};func TestStream(t *testing.T){a,b:=&stream{},&stream{};x,y:=NewOutput(a),NewOutput(b);if n,e:=x.Move(7);n!=7||e!=nil{t.Fatal(n,e)};if n,e:=y.Move(2);n!=2||e!=nil{t.Fatal(n,e)};a.failure=errors.New("seek failed");if n,e:=x.Move(3);n!=10||e!=a.failure||b.off!=2{t.Fatal(n,e,b.off)}}`)
	cmd := exec.Command("go", "test", "-race", "./...")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOWORK=off", "GOTOOLCHAIN=go1.27.1")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	for name, changed := range map[string]string{"unreviewed method": source + `;func(o *Output)Close()error{return o.file.Close()}`, "unreviewed file factory": source + `;func Bad(){os.Open("file")}`, "wrong field type": strings.ReplaceAll(strings.Replace(source, `"os"`, `"bytes"`, 1), "*os.File", "*bytes.Reader")} {
		t.Run(name, func(t *testing.T) {
			s.cache[path] = []byte(changed)
			if _, err := Render(s, plan, dir); err == nil {
				t.Fatal("invalid host field accepted")
			}
		})
	}
	for _, rule := range []HostFieldRule{{Type: "Output", Field: "File", Methods: []string{"Seek"}}, {Type: "Output", Field: "file", Methods: []string{"Stat"}}, {Type: "Output", Field: "file", Methods: []string{"Seek", "Seek"}}} {
		p := plan
		p.Units = append([]Unit(nil), plan.Units...)
		p.Units[0].HostFields = []HostFieldRule{rule}
		if err := p.Validate(); err == nil {
			t.Fatal("invalid rule accepted", rule)
		}
	}
}

package port

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestNativeHostStreamSignatures(t *testing.T) {
	const path = "src/cmd/compile/internal/p/p.go"
	const source = `package p;import "os";type Input struct{file *os.File};func New(f *os.File)*Input{return &Input{file:f}};func(i *Input)File()*os.File{return i.file};func(i *Input)Read(b []byte)(int,error){return i.file.Read(b)}`
	repo, pin := fixtureRepo(t, map[string]string{"LICENSE": "fixture", "PATENTS": "fixture", path: source})
	s, e := OpenSource(context.Background(), repo, pin)
	if e != nil {
		t.Fatal(e)
	}
	methods := []string{"Read", "ReadAt", "Seek", "Close"}
	plan := Plan{InspectRoots: []string{"cmd/compile/internal/p"}, Units: []Unit{{Source: path, WholeFile: true, HostFields: []HostFieldRule{{Type: "Input", Field: "file", Methods: methods}}, HostSignatures: []HostSignatureRule{{Function: "New", Methods: methods}, {Function: "Input.File", Result: true, Methods: methods}}}}}
	dir := t.TempDir()
	r, e := Render(s, plan, dir)
	if e != nil {
		t.Fatal(e)
	}
	if !strings.Contains(string(r.Files["internal/p/p.go"]), "ReadAt") {
		t.Fatal("missing stream capability")
	}
	recorded := 0
	for _, entry := range r.Ledger.Files {
		for _, decl := range entry.Declarations {
			if strings.HasPrefix(decl, "host-stream-signature:") {
				recorded++
			}
		}
	}
	if recorded != 2 {
		t.Fatal("missing whole-file signature provenance", recorded)
	}
	if e = Install(dir, r); e != nil {
		t.Fatal(e)
	}
	writeTestFile(t, dir, "go.mod", "module go-backend-ssa\n\ngo 1.27.1\n")
	writeTestFile(t, dir, "internal/p/check_test.go", `package p
import("bytes";"errors";"fmt";"sync";"testing")
type stream struct{*bytes.Reader;err error};func(s *stream)Close()error{return s.err}
func TestStreams(t *testing.T){var wg sync.WaitGroup;for i:=range 8{wg.Go(func(){s:=&stream{bytes.NewReader([]byte(fmt.Sprint(i))),errors.New("close")};r:=New(s);b:=make([]byte,1);if n,e:=r.Read(b);n!=1||e!=nil||string(b)!=fmt.Sprint(i){t.Error(n,e,string(b))};if r.File()!=s||r.File().Close()!=s.err{t.Error("identity")};if n,e:=r.File().Seek(0,0);n!=0||e!=nil{t.Error(n,e)};if n,e:=r.File().ReadAt(b,0);n!=1||e!=nil{t.Error(n,e)}})};wg.Wait()}
`)
	cmd := exec.Command("go", "test", "-race", "./...")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOWORK=off", "GOTOOLCHAIN=go1.27.1")
	if b, e := cmd.CombinedOutput(); e != nil {
		t.Fatalf("%v\n%s", e, b)
	}
	for name, changed := range map[string]string{
		"unreviewed operation":        source + `;func Bad(f *os.File)uintptr{return f.Fd()}`,
		"unreviewed projected method": strings.Replace(source, "return i.file.Read(b)", "i.file.Sync();return i.file.Read(b)", 1),
		"grouped parameters":          strings.Replace(source, "New(f *os.File)", "New(f,g *os.File)", 1),
		"wrong input type":            strings.Replace(source, "New(f *os.File)", "New(f any)", 1),
		"missing result":              strings.Replace(source, "File()*os.File{return i.file}", "File(){}", 1),
	} {
		t.Run(name, func(t *testing.T) {
			s.cache[path] = []byte(changed)
			defer func() { s.cache[path] = []byte(source) }()
			if _, e := Render(s, plan, dir); e == nil {
				t.Fatal("invalid projection accepted")
			}
		})
	}
	for _, rule := range []HostSignatureRule{{Function: "New", Index: -1, Methods: methods}, {Function: "New", Methods: []string{"Fd"}}, {Function: "New", Methods: []string{"Read", "Read"}}, {Function: "New"}} {
		p := plan
		p.Units = append([]Unit(nil), plan.Units...)
		p.Units[0].HostSignatures = []HostSignatureRule{rule}
		if p.Validate() == nil {
			t.Fatal("invalid rule accepted", rule)
		}
	}
	p := plan
	p.Units = append([]Unit(nil), plan.Units...)
	p.Units[0].HostSignatures = append(append([]HostSignatureRule(nil), plan.Units[0].HostSignatures...), plan.Units[0].HostSignatures[0])
	if p.Validate() == nil {
		t.Fatal("duplicate rule accepted")
	}
	p = plan
	p.Units = append([]Unit(nil), plan.Units...)
	p.Units[0].HostSignatures = []HostSignatureRule{{Function: "New", Index: 5, Methods: methods}}
	if _, e := Render(s, p, dir); e == nil {
		t.Fatal("missing slot accepted")
	}
}

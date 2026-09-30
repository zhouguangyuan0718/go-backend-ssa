package port

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestReviewedFixedSizeof(t *testing.T) {
	const path = "src/cmd/compile/internal/p/p.go"
	const code = `package p;import "unsafe";type mask struct{a,b uint64};func Bits()int{return int(unsafe.Sizeof(mask{})*8)}`
	repo, pin := fixtureRepo(t, map[string]string{"LICENSE": "fixture", "PATENTS": "fixture", path: code})
	s, err := OpenSource(context.Background(), repo, pin)
	if err != nil {
		t.Fatal(err)
	}
	plan := Plan{InspectRoots: []string{"cmd/compile/internal/p"}, Units: []Unit{{Source: path, WholeFile: true, FixedSizeof: []FixedSizeofRule{{Function: "Bits", Type: "mask"}}}}}
	dir := t.TempDir()
	r, err := Render(s, plan, dir)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(r.Files["internal/p/p.go"]), "unsafe.Sizeof(mask{})") {
		t.Fatal("native size observation changed")
	}
	if err := Install(dir, r); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, dir, "go.mod", "module go-backend-ssa\n\ngo 1.27.1\n")
	writeTestFile(t, dir, "internal/p/adapter_test.go", `package p;import "testing";func TestBits(t *testing.T){if Bits()!=128{t.Fatal(Bits())}}`)
	cmd := exec.Command("go", "test", "./...")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOWORK=off", "GOTOOLCHAIN=go1.27.1")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	for name, body := range map[string]string{
		"native int":            strings.Replace(code, "a,b uint64", "a,b int", 1),
		"pointer":               strings.Replace(code, "a,b uint64", "a,b *uint64", 1),
		"slice":                 strings.Replace(code, "a,b uint64", "a,b []uint64", 1),
		"padding":               strings.Replace(code, "a,b uint64", "a byte;b uint64", 1),
		"address":               strings.Replace(code, "mask{}", "&mask{}", 1),
		"alignment":             strings.Replace(code, "unsafe.Sizeof", "unsafe.Alignof", 1),
		"second observation":    strings.Replace(code, "*8)", "*8+unsafe.Sizeof(int(0)))", 1),
		"duplicate observation": strings.Replace(code, "*8)", "*8+unsafe.Sizeof(mask{}))", 1),
		"nonempty literal":      strings.Replace(code, "mask{}", "mask{a:1}", 1),
	} {
		t.Run(name, func(t *testing.T) {
			s.cache[path] = []byte(body)
			if _, err := Render(s, plan, dir); err == nil {
				t.Fatal("unreviewed layout observation accepted")
			}
		})
	}
	s.cache[path] = []byte(code)
	for _, rules := range [][]FixedSizeofRule{{{Function: "", Type: "mask"}}, {{Function: "Bits", Type: "*mask"}}, {{Function: "Bits", Type: "mask"}, {Function: "Bits", Type: "mask"}}} {
		bad := plan
		bad.Units = append([]Unit(nil), plan.Units...)
		bad.Units[0].FixedSizeof = rules
		if _, err := Render(s, bad, dir); err == nil {
			t.Fatal("invalid sizeof rules accepted")
		}
	}
	plan.Units[0].FixedSizeof = nil
	if _, err := Render(s, plan, dir); err == nil {
		t.Fatal("unclassified sizeof accepted")
	}
}

package port

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestWholePackageReceiversAndFreshZeroValues(t *testing.T) {
	files := map[string]string{
		"LICENSE": "fixture", "PATENTS": "fixture",
		"src/cmd/compile/internal/p/a.go": `package p
type Pos struct { N int }
var absent Pos
func (p *Pos) init(n int) { p.N=n }
func (p Pos) Number() int {return p.N}
`,
		"src/cmd/compile/internal/p/b.go": `package p
func Missing() *Pos {return &absent}
func IsAbsent(p Pos) bool {if p == absent {return true}; return false}
func Shadow() int { absent:=Pos{4}; return absent.N }
func New(n int) Pos {var p Pos; p.init(n); return p}
`,
	}
	repo, pin := fixtureRepo(t, files)
	s, err := OpenSource(context.Background(), repo, pin)
	if err != nil {
		t.Fatal(err)
	}
	plan := Plan{InspectRoots: []string{"cmd/compile/internal/p"}, Units: []Unit{
		{Source: "src/cmd/compile/internal/p/a.go", WholeFile: true, ZeroValues: []string{"absent"}},
		{Source: "src/cmd/compile/internal/p/b.go", WholeFile: true},
	}}
	dir := t.TempDir()
	r, err := Render(s, plan, dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := Install(dir, r); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, dir, "go.mod", "module go-backend-ssa\n\ngo 1.27.1\n")
	writeTestFile(t, dir, "internal/p/adapter_test.go", `package p
import "testing"
func TestValues(t *testing.T) {
 a,b:=Missing(),Missing();a.N=99
 if b.N!=0 || Missing().N!=0 || Shadow()!=4 || !IsAbsent(*b) {t.Fatal("shared zero value or local shadow rewritten")}
 method:=Pos.Number
 if method(New(7))!=7 {t.Fatal("receiver/method expression changed")}
}
`)
	cmd := exec.Command("go", "test", "-race", "./...")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOWORK=off", "GOTOOLCHAIN=go1.27.1")
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%v\n%s", err, b)
	}

	for _, tc := range []struct{ name, source, want string }{
		{"initializer", "package p; type Pos struct{N int}; var absent=Pos{1}", "uninitialized"},
		{"unclassified state", "package p; type Pos struct{N int}; var absent Pos; var cache=map[int]int{}", "remaining package variable"},
		{"package init", "package p; type Pos struct{N int}; var absent Pos; func init(){}", "package init requires"},
		{"assignment", "package p; type Pos struct{N int}; var absent Pos; func Reset(){absent=Pos{}}", "transformed package"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			source := tc.source + "\nfunc (p *Pos) init(n int){p.N=n}; func(p Pos) Number()int{return p.N}\n"
			// Simulate a new immutable blob read without changing any worktree.
			old := s.cache[plan.Units[0].Source]
			s.cache[plan.Units[0].Source] = []byte(source)
			defer func() { s.cache[plan.Units[0].Source] = old }()
			if _, err := Render(s, plan, t.TempDir()); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("want %q, got %v", tc.want, err)
			}
		})
	}
}

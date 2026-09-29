package port

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestReadTableLocalizationAndRejections(t *testing.T) {
	source := `package p
var index=[...]uint8{0,3,8}
var names=[...]string{"invalid","int","string"}
type Kind int
func(k Kind)String()string{if k<0||int(k)>=len(names){return "unknown"};return names[k]}
func Read(i int)uint8{return index[i]}
func Count()int{return len(index)}
func Closure()func()uint8{return func()uint8{return index[2]}}
func Shadow()int{index:=[]int{7};return index[0]}
`
	repo, pin := fixtureRepo(t, map[string]string{"LICENSE": "fixture", "PATENTS": "fixture", "src/cmd/compile/internal/p/p.go": source})
	s, err := OpenSource(context.Background(), repo, pin)
	if err != nil {
		t.Fatal(err)
	}
	plan := Plan{InspectRoots: []string{"cmd/compile/internal/p"}, Units: []Unit{{Source: "src/cmd/compile/internal/p/p.go", WholeFile: true, ReadTables: []string{"index", "names"}}}}
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
func TestRead(t *testing.T){if Read(1)!=3||Count()!=3||Closure()()!=8||Shadow()!=7||Kind(2).String()!="string"||Kind(-1).String()!="unknown"{t.Fatal("read semantics changed")}}
`)
	cmd := exec.Command("go", "test", "-race", "./...")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOWORK=off", "GOTOOLCHAIN=go1.27.1")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	for name, extra := range map[string]string{
		"string write": "func Bad(){names[0]=\"changed\"}",
		"write":        "func Bad(){index[0]=9}", "paren write": "func Bad(){(index[0])=9}", "increment": "func Bad(){index[0]++}",
		"address": "func Bad()*uint8{return &(index[0])}", "slice": "func Bad()[]uint8{return index[:]}", "copy": "func Bad()[3]uint8{return index}",
	} {
		t.Run(name, func(t *testing.T) {
			s.cache[plan.Units[0].Source] = []byte(source + extra)
			if _, err := Render(s, plan, dir); err == nil || !strings.Contains(err.Error(), "unsupported escape") {
				t.Fatalf("unsafe table accepted: %v", err)
			}
		})
	}
}

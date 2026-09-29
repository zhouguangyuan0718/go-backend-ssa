package port

import (
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Compile the same upstream slice twice: once with its original globals/init,
// and once with the library's state conversion. Only import paths are relocated
// in the reference. Comparing observable snapshots catches initializer and
// identity regressions without reproducing the algorithms in the test.
func testUpstreamSSAState(t *testing.T, source *Source, rendered *Rendered, root string) {
	t.Helper()
	dir := t.TempDir()
	for name, data := range rendered.Files {
		writeTestFile(t, dir, name, string(data))
	}
	plan := Migration()
	for _, u := range plan.Units {
		if u.Context != nil {
			data, err := os.ReadFile(filepath.Join(root, u.adapterPath()))
			if err != nil {
				t.Fatal(err)
			}
			writeTestFile(t, dir, u.adapterPath(), string(data))
		}
		if u.sourcePackage() != "cmd/compile/internal/types" {
			continue
		}
		data, err := source.Read(u.Source)
		if err != nil {
			t.Fatal(err)
		}
		fs := token.NewFileSet()
		f, err := selectFile(fs, u.Source, data, u)
		if err != nil {
			t.Fatal(err)
		}
		// Deliberately bypass context/owner rewriting and the no-global output audit.
		raw, err := transform(fs, f, nil, Unit{Source: u.Source}, plan.importMap(source.Pin.TargetModule))
		if err != nil {
			t.Fatal(err)
		}
		writeTestFile(t, dir, "internal/reference/types/"+filepath.Base(u.Source), string(raw))
	}
	snapshot := `package types
import "fmt"
func Snapshot() []string {
 SETUP
 var out []string
 for _,typ:=range []*Type{UP.TypeInvalid,UP.TypeMem,UP.TypeFlags,UP.TypeVoid,UP.TypeInt128,UP.TypeVec128,UP.TypeVec256,UP.TypeVec512,UP.TypeMask} {
  out=append(out,fmt.Sprintf("%v:%d:%d:%d:%t:%t:%t:%t:%t",typ.extra,typ.kind,typ.width,typ.align,typ.IsMemory(),typ.IsFlags(),typ.IsVoid(),typ.IsSIMD(),typ.Underlying()==typ))
 }
 value:=UP.newType(TINT)
 value.SetHasShape(true)
 for _,typ:=range []*Type{UP.NewTuple(value,UP.TypeMem),UP.NewTuple(UP.TypeMem,value),UP.NewResults(nil),UP.NewResults([]*Type{value,UP.TypeMem}),UP.NewResults([]*Type{UP.TypeMem,value}),UP.NewResults([]*Type{UP.TypeMem})} {
  out=append(out,fmt.Sprintf("%d:%t:%t:%t:%t:%t",typ.kind,typ.IsMemory(),typ.IsTuple(),typ.IsResults(),typ.HasShape(),typ==UP.TypeResultMem))
 }
 cached:=UP.TypeResultMem.extra.(*Results).Types
 out=append(out,fmt.Sprintf("%d:%t",len(cached),cached[0]==UP.TypeMem))
 return out
}
`
	for _, variant := range []struct{ path, setup, prefix string }{
		{"internal/reference/types", "", ""}, {"internal/types", "u:=NewUniverse(8,8,1<<50)", "u."},
	} {
		code := strings.ReplaceAll(strings.ReplaceAll(snapshot, "SETUP", variant.setup), "UP.", variant.prefix)
		writeTestFile(t, dir, variant.path+"/adapter_snapshot.go", code)
	}
	writeTestFile(t, dir, "go.mod", "module go-backend-ssa\n\ngo 1.27.1\n")
	writeTestFile(t, dir, "compare_test.go", `package comparison_test
import (
 "reflect"
 "testing"
 native "go-backend-ssa/internal/reference/types"
 library "go-backend-ssa/internal/types"
)
func TestUpstreamSSAState(t *testing.T){
 want:=native.Snapshot()
 for i:=0;i<3;i++{if got:=library.Snapshot();!reflect.DeepEqual(got,want){t.Fatalf("library %v\nupstream %v",got,want)}}
}
`)
	cmd := exec.Command("go", "test", "-race", "-v", ".")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOWORK=off", "GOTOOLCHAIN=go1.27.1")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("upstream SSA differential (%s): %v\n%s", source.Pin.Tag, err, out)
	}
	t.Logf("upstream SSA differential at %s:\n%s", source.Pin.Tag, out)
}

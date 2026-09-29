package port

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func transformFixture(t *testing.T, source string, u Unit) ([]byte, error) {
	t.Helper()
	fs := token.NewFileSet()
	f, e := selectFile(fs, "fixture.go", []byte(source), u)
	if e != nil {
		return nil, e
	}
	_, info, e := checkTypes(fs, "fixture", []*ast.File{f}, newImporter())
	if e != nil {
		return nil, e
	}
	return transform(fs, f, info, u, nil)
}

// Execute the rewritten code: parsing/string matching alone misses callback,
// shadowing, initialization-order and cross-context behavior changes.
func TestContextRewriteSemantics(t *testing.T) {
	source := `package fixture
import "strconv"
var counter = 3
var alias = counter
var table = map[int]int{1: 7}
var label = strconv.Itoa(counter)
func Add(n int) int { counter += n; return counter }
func Callback() func(int) int { return Add }
func Closure() func() int { return func() int { return Add(table[1]) } }
func Shadow(counter int) int { return counter + alias }
func Recursive(n int) int { if n == 0 { return counter }; return Recursive(n-1) }
func InitAlias() int { return alias }
func Label() string { return label }
`
	u := Unit{Functions: []string{"Add", "Callback", "Closure", "Shadow", "Recursive", "InitAlias", "Label"}, Context: &ContextRule{Type: "Context", Constructor: "newContext", Fields: map[string]string{"counter": "count", "alias": "initial", "table": "table", "label": "label"}}}
	out, err := transformFixture(t, source, u)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	writeTestFile(t, dir, "go.mod", "module fixture\n\ngo 1.27.1\n")
	writeTestFile(t, dir, "generated.go", string(out))
	writeTestFile(t, dir, "context.go", "package fixture\ntype Context struct { count int; initial int; table map[int]int; label string }\n")
	writeTestFile(t, dir, "semantics_test.go", `package fixture
import "testing"
func TestBehavior(t *testing.T) {
 a,b:=newContext(),newContext()
 if a.InitAlias()!=3 || b.InitAlias()!=3 || a.Label()!="3" {t.Fatal("initializer dependency changed")}
 callback, closure:=a.Callback(),a.Closure()
 if callback(2)!=5 || closure()!=12 || a.Recursive(4)!=12 {t.Fatal("call/closure/recursive context lost")}
 if b.Add(0)!=3 || b.Shadow(20)!=23 {t.Fatal("context leak or shadowed local rewritten")}
 a.table[1]=100
 if b.Closure()()!=10 {t.Fatal("shared map initializer")}
}
`)
	cmd := exec.Command("go", "test", "-race", ".")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOTOOLCHAIN=go1.27.1", "GOWORK=off")
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("execute generated fixture: %v\n%s\n%s", err, b, out)
	}
}

func TestRewriteRejectsUnknownShapes(t *testing.T) {
	for _, tc := range []struct {
		name, source string
		functions    []string
		fields       map[string]string
		want         string
	}{
		{"missing", "package p; func F() int{return 1}", []string{"Missing"}, nil, "not found"},
		{"unselected dependency", "package p; var n int; func F() int{return n}", []string{"F"}, nil, "undefined: n"},
		{"tuple init", "package p; var a,b = 1,2; func F() int{return a+b}", []string{"F"}, map[string]string{"a": "a", "b": "b"}, "unsupported state"},
		{"generic", "package p; func F[T any](x T) T{return x}", []string{"F"}, nil, "unsupported function shape"},
		{"receiver", "package p; type T int; func (T) F() int{return 1}", []string{"F"}, nil, "selected declaration not found"},
		{"collision", "package p; var n int; func F(syncgoCtx int) int{return n+syncgoCtx}", []string{"F"}, map[string]string{"n": "n"}, "collision"},
		{"package init", "package p; var n int; func init(){n=9}; func F() int{return n}", []string{"F"}, map[string]string{"n": "n"}, "package init requires"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			u := Unit{Functions: tc.functions}
			if tc.fields != nil {
				u.Context = &ContextRule{Type: "Context", Constructor: "newContext", Fields: tc.fields}
			}
			_, err := transformFixture(t, tc.source, u)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("want %q, got %v", tc.want, err)
			}
		})
	}
}

func TestResidualInternalImportRejected(t *testing.T) {
	fs := token.NewFileSet()
	f, err := parser.ParseFile(fs, "x.go", `package p; import "internal/abi"; func F() { abi.F() }`, 0)
	if err != nil {
		t.Fatal(err)
	}
	_, err = transform(fs, f, nil, Unit{}, nil)
	if err == nil || !strings.Contains(err.Error(), "residual upstream internal import") {
		t.Fatal(err)
	}
}

func writeTestFile(t *testing.T, root, name, contents string) {
	t.Helper()
	p := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(contents), 0644); err != nil {
		t.Fatal(err)
	}
}

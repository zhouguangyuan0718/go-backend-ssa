package syntax_test

import (
	"bytes"
	"errors"
	"fmt"
	"go-backend-ssa/syntax"
	"io"
	"strings"
	"sync"
	"testing"
)

const sample = `//go:build go1.27
package p
//go:noinline
func Sum[T ~int](xs []T) T {
 var sum T
Loop:
 for _,x := range xs { if x<0 {continue Loop}; sum+=x }
 return sum
}
`

func TestNativeTreeAndBranchTargets(t *testing.T) {
	var directives []string
	f, err := syntax.Parse(syntax.NewFileBase("input.go"), strings.NewReader(sample), nil, func(pos syntax.Pos, blank bool, text string, current syntax.Pragma) syntax.Pragma {
		directives = append(directives, text)
		return text
	}, syntax.CheckBranches)
	if err != nil {
		t.Fatal(err)
	}
	if f.PkgName.Value != "p" || f.GoVersion != "go1.27" || len(f.DeclList) != 1 {
		t.Fatal(f)
	}
	fn := f.DeclList[0].(*syntax.FuncDecl)
	if len(fn.TParamList) != 1 || fn.Pragma != "go:noinline" || len(directives) != 2 {
		t.Fatal(fn, directives)
	}
	labeled := fn.Body.List[1].(*syntax.LabeledStmt)
	loop := labeled.Stmt.(*syntax.ForStmt)
	var branch *syntax.BranchStmt
	syntax.Inspect(f, func(n syntax.Node) bool {
		if n, ok := n.(*syntax.BranchStmt); ok {
			branch = n
		}
		return true
	})
	if branch == nil || branch.Target != loop || branch.Tok != syntax.Continue {
		t.Fatal("branch lost target identity")
	}
	if syntax.StartPos(fn).Line() != 4 || syntax.EndPos(fn).Line() != 9 {
		t.Fatal(syntax.StartPos(fn), syntax.EndPos(fn))
	}
	var printed, dump bytes.Buffer
	if _, err := syntax.Fprint(&printed, f, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := syntax.Parse(syntax.NewFileBase("print.go"), strings.NewReader(printed.String()), nil, nil, syntax.CheckBranches); err != nil {
		t.Fatal(err)
	}
	if err := syntax.Fdump(&dump, f); err != nil || !strings.Contains(dump.String(), "FuncDecl") {
		t.Fatal(err, dump.String())
	}
}

func TestErrorsDirectivesAndReentrantParse(t *testing.T) {
	const bad = "package p; func f() { break }"
	f, err := syntax.Parse(syntax.NewFileBase("bad.go"), strings.NewReader(bad), nil, nil, syntax.CheckBranches)
	if f != nil || err == nil {
		t.Fatal("first-error contract", f, err)
	}
	var issues []error
	nested := false
	f, err = syntax.Parse(syntax.NewFileBase("bad.go"), strings.NewReader(bad), func(err error) {
		issues = append(issues, err)
		inside, e := syntax.Parse(syntax.NewFileBase("good.go"), strings.NewReader("package q; var x int"), nil, nil, 0)
		if e != nil || inside.PkgName.Value != "q" {
			t.Fatal(inside, e)
		}
		nested = true
	}, nil, syntax.CheckBranches)
	if f == nil || err == nil || !nested || len(issues) != 1 || !strings.Contains(issues[0].Error(), "break is not in a loop") {
		t.Fatal(f, err, issues)
	}
	// Reusing the same filename never reuses buffers, nodes or source bases.
	oldName := f.PkgName
	lineTree, err := syntax.Parse(syntax.NewFileBase("bad.go"), strings.NewReader("package p\n//line remapped.go:40\nvar x int\n"), nil, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	decl := lineTree.DeclList[0].(*syntax.VarDecl)
	if decl.Pos().RelFilename() != "remapped.go" || decl.Pos().RelLine() != 40 || oldName != f.PkgName || oldName.Pos().Base() == lineTree.PkgName.Pos().Base() {
		t.Fatal("position or old tree identity", decl.Pos())
	}
	marker := errors.New("callback panic")
	func() {
		defer func() {
			if recover() != marker {
				t.Error("callback panic swallowed")
			}
		}()
		syntax.Parse(nil, strings.NewReader("invalid"), func(error) { panic(marker) }, nil, 0)
	}()
}

type failedReader struct{}

func (failedReader) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }
func TestReaderErrorAndRecovery(t *testing.T) {
	if f, err := syntax.Parse(syntax.NewFileBase("read.go"), failedReader{}, nil, nil, 0); f != nil || err == nil || !strings.Contains(err.Error(), "unexpected EOF") {
		t.Fatal(f, err)
	}
	if _, err := syntax.Parse(nil, strings.NewReader("package p"), nil, nil, 0); err != nil {
		t.Fatal(err)
	}
}

func TestConcurrentAndNestedPragmas(t *testing.T) {
	var wg sync.WaitGroup
	for i := range 24 {
		wg.Go(func() {
			file := fmt.Sprintf("%d.go", i)
			calls := 0
			f, err := syntax.Parse(syntax.NewFileBase(file), strings.NewReader(sample), nil, func(pos syntax.Pos, blank bool, text string, current syntax.Pragma) syntax.Pragma {
				calls++
				g, e := syntax.Parse(syntax.NewFileBase(file), strings.NewReader("package inner"), nil, nil, 0)
				if e != nil || g.PkgName.Value != "inner" {
					t.Error(g, e)
				}
				return i
			}, syntax.CheckBranches)
			if err != nil || calls != 2 || f.DeclList[0].(*syntax.FuncDecl).Pragma != i {
				t.Error(f, err, calls)
				return
			}
			for range 5 {
				var b bytes.Buffer
				if _, err := syntax.Fprint(&b, f, syntax.LineForm); err != nil {
					t.Error(err)
				}
				if _, err := syntax.Parse(syntax.NewFileBase(file), strings.NewReader(b.String()), nil, nil, syntax.CheckBranches); err != nil {
					t.Error(err)
				}
			}
		})
	}
	wg.Wait()
}

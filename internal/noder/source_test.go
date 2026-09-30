package noder

import (
	"bytes"
	"fmt"
	"go-backend-ssa/internal/ir"
	"go-backend-ssa/internal/syntax"
	"io"
	"io/fs"
	"os"
	"strings"
	"testing"
)

type syntaxOutput struct {
	bytes.Buffer
	closed int
}

func (w *syntaxOutput) Close() error { w.closed++; return nil }
func parseSource(t *testing.T, c *Context, name, source string) *ParsedFile {
	t.Helper()
	f, e := c.ParseFile(name, strings.NewReader(source))
	if e != nil {
		t.Fatal(e)
	}
	return f
}
func TestNativeSourceFrontend(t *testing.T) {
	archives := dependencyArchives(t)
	for i := range 8 {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			t.Parallel()
			c, opens, closes := dependencyContext(t, archives, true)
			oldChecker := c.Checker
			oldInt := oldChecker.Universe.Lookup("int")
			c.Debug.AstDump = "F"
			reports := map[string]*syntaxOutput{}
			c.OpenSyntaxHTML = func(p string, flags int, mode fs.FileMode) (io.WriteCloser, error) {
				if flags != os.O_WRONLY|os.O_CREATE|os.O_TRUNC || mode != 0644 {
					t.Fatal("native output flags changed")
				}
				w := new(syntaxOutput)
				reports[p] = w
				return w, nil
			}
			c.SyntaxWorkingDirectory = func() (string, error) { return "/reports", nil }
			var trace bytes.Buffer
			c.Typecheck.IR.TraceWriter = &trace
			f := parseSource(t, c, "/src/a.go", `package p;import "example/a";import "example/b";func Each(y func(int)bool){for i:=0;i<4;i++{if !y(i){return}}};type Box[T any]struct{V T};func Id[T any](v T)T{return v};func F()(n int){for v:=range Each{n+=v};return n+a.Add(1)+b.Sum(2)+Id(Box[int]{V:3}).V};var G=a.Global`)
			extra := parseSource(t, c, "/src/b.go", "package p\n//go:noinline\nfunc Other(x int)int{return F()+x}")
			target, e := c.CompileParsedPackage([]*ParsedFile{f, extra})
			if e != nil {
				t.Fatal(e)
			}
			if target != c.Typecheck.Target || len(target.Funcs) == 0 || c.localPkgReader == nil {
				t.Fatal("missing native package")
			}
			if c.Checker == oldChecker || oldChecker.Universe.Lookup("int") != oldInt || c.anyTypeName != c.Checker.Universe.Lookup("any") {
				t.Fatal("checker/writer universe lifecycle mismatch")
			}
			if *closes != 2 || len(*opens) != 2 {
				t.Fatal("dependency reads", *opens, *closes)
			}
			for _, fn := range target.Funcs {
				if fn.Typecheck() == 0 || len(fn.Body) > 0 && fn.Body[0].Typecheck() == 0 {
					t.Fatal("native unified left unchecked IR")
				}
			}
			if c.Types.LocalPkg.Lookup("Other").Def.(*ir.Name).Func.Pragma&ir.Noinline == 0 {
				t.Fatal("directive lost")
			}
			syntax.Inspect(f.Syntax(), func(n syntax.Node) bool {
				if loop, ok := n.(*syntax.ForStmt); ok {
					if r, ok := loop.Init.(*syntax.RangeClause); ok && strings.Contains(syntax.String(r.X), "Each") {
						t.Fatal("rangefunc not rewritten")
					}
				}
				return true
			})
			if len(reports) != 1 {
				t.Fatal("missing syntax report", len(reports))
			}
			for _, w := range reports {
				if w.closed != 1 || !strings.Contains(w.String(), "rangefunc") || !strings.Contains(w.String(), "midway after recheck") || !strings.HasSuffix(w.String(), "</html>\n") {
					t.Fatal("incomplete native HTML")
				}
			}
			if len(c.syntaxHTMLWriters) != 0 || len(c.syntaxOrderedFuncs) != 0 || !strings.Contains(trace.String(), "Writing html syntax output") {
				t.Fatal("HTML cleanup missing")
			}
			c.Interleaved.DevirtualizeAndInlinePackage(target, nil)
			if c.Diagnostics.HasErrors() {
				t.Fatal(c.Diagnostics.Diagnostics())
			}
			if _, e := c.CompileParsedPackage([]*ParsedFile{f}); e == nil {
				t.Fatal("consumed source context accepted")
			}
		})
	}
}
func TestSourceFrontendFailures(t *testing.T) {
	for _, test := range []struct{ source, lang, want string }{
		{"package p;var X int=\"bad\"", "go1.27", "cannot use"},
		{"package p;type T = interface{M() T}", "go1.27", "invalid recursive type"},
		{"package p;func F(){for range 3{}}", "go1.21", "-lang was set to go1.21"},
		{"//go:build go1.21\npackage p;func F(){for range 3{}}", "go1.27", "file declares //go:build go1.21"},
		{"package p;import \"missing/dependency\";var _=dependency.X", "go1.27", "could not import"},
	} {
		t.Run(test.want, func(t *testing.T) {
			c, _, _ := dependencyContext(t, nil, false)
			c.Flag.Lang = test.lang
			c.Flag.ErrorURL = true
			f := parseSource(t, c, "/src/fail.go", test.source)
			target, e := c.CompileParsedPackage([]*ParsedFile{f})
			if target != nil || e == nil || !strings.Contains(e.Error(), test.want) {
				t.Fatalf("got %v, %v", target, e)
			}
			diagnostics, ok := e.(SourceErrors)
			if !ok || len(diagnostics) == 0 {
				t.Fatalf("diagnostics not preserved: %T", e)
			}
			if _, e := c.CompileParsedPackage([]*ParsedFile{f}); e == nil {
				t.Fatal("failed context reused")
			}
		})
	}
}
func TestSourceFrontendPreflightAndProviderPanic(t *testing.T) {
	c, _, _ := dependencyContext(t, nil, false)
	f := parseSource(t, c, "a.go", "package p;func F(){}")
	other, _, _ := dependencyContext(t, nil, false)
	q := parseSource(t, other, "q.go", "package p")
	for _, files := range [][]*ParsedFile{nil, {nil}, {q}, {f, f}} {
		if _, e := c.CompileParsedPackage(files); e == nil {
			t.Fatal("invalid source accepted")
		}
		if c.sourceStarted {
			t.Fatal("preflight consumed context")
		}
	}
	c.Debug.AstDump = "F"
	if _, e := c.CompileParsedPackage([]*ParsedFile{f}); e == nil {
		t.Fatal("unbound report provider accepted")
	}
	c.Debug.AstDump = ""
	sentinel := new(int)
	c.dependencyOpen = func(string) (DependencyFile, error) { panic(sentinel) }
	f = parseSource(t, c, "panic.go", "package p;import \"missing/dependency\";var _=dependency.X")
	func() {
		defer func() {
			if recover() != sentinel {
				t.Error("provider panic changed")
			}
		}()
		c.CompileParsedPackage([]*ParsedFile{f})
	}()
	if !c.sourceStarted || c.sourceMode {
		t.Fatal("failed source lifecycle not retained")
	}
}

type panickingSyntaxClose struct {
	syntaxOutput
	failure any
}

func (w *panickingSyntaxClose) Close() error { w.closed++; panic(w.failure) }
func TestSourceHTMLFailureUnwinds(t *testing.T) {
	c, _, _ := dependencyContext(t, nil, false)
	c.Debug.AstDump = "~example/p\\."
	first, cleanup := new(int), new(int)
	writer := &panickingSyntaxClose{failure: cleanup}
	opens := 0
	c.OpenSyntaxHTML = func(string, int, fs.FileMode) (io.WriteCloser, error) {
		opens++
		if opens > 1 {
			panic(first)
		}
		return writer, nil
	}
	c.SyntaxWorkingDirectory = func() (string, error) { return "/reports", nil }
	f := parseSource(t, c, "input.go", "package p;func F(){};func G(){}")
	func() {
		defer func() {
			if p := recover(); p != first {
				t.Errorf("primary provider panic changed: %v", p)
			}
		}()
		c.CompileParsedPackage([]*ParsedFile{f})
	}()
	if writer.closed != 1 || c.sourceMode || !c.sourceStarted {
		t.Fatal("report cleanup or source lifecycle failed")
	}
}

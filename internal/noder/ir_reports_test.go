package noder

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"strings"
	"testing"
)

type irReportFile struct {
	bytes.Buffer
	opens, closes int
	flags         []int
}
type irReportHandle struct {
	file       *irReportFile
	closed     bool
	closeError error
}

func (h *irReportHandle) Write(p []byte) (int, error) {
	if h.closed {
		return 0, errors.New("closed report")
	}
	return h.file.Write(p)
}
func (h *irReportHandle) Close() error {
	if h.closed {
		return errors.New("report closed twice")
	}
	h.closed = true
	h.file.closes++
	return h.closeError
}
func bindIRReports(t *testing.T, c *Context) map[string]*irReportFile {
	t.Helper()
	files := map[string]*irReportFile{}
	c.Typecheck.IR.OpenAST = func(path string, flags int, mode fs.FileMode) (io.WriteCloser, error) {
		f := files[path]
		if f == nil {
			f = new(irReportFile)
			files[path] = f
		}
		if strings.HasSuffix(path, ".ast") {
			want := os.O_APPEND | os.O_RDWR
			if f.opens == 0 {
				want = os.O_CREATE | os.O_TRUNC | os.O_RDWR
			}
			if flags != want || mode != 0777 {
				t.Fatalf("text flags: %s %d %o", path, flags, mode)
			}
		} else if flags != os.O_WRONLY|os.O_CREATE|os.O_TRUNC || mode != 0644 {
			t.Fatalf("HTML flags: %s %d %o", path, flags, mode)
		}
		if flags&os.O_TRUNC != 0 {
			f.Reset()
		}
		f.opens++
		f.flags = append(f.flags, flags)
		return &irReportHandle{file: f}, nil
	}
	c.Typecheck.IR.ASTWorkingDirectory = func() (string, error) { return "/reports", nil }
	return files
}
func reportSourceContext(t *testing.T) *Context {
	c, _, _ := dependencyContext(t, nil, false)
	f := parseSource(t, c, "reports.go", `package p;func G(x int)int{return x+1};func F(x int)int{return G(x)}`)
	if _, e := c.CompileParsedPackage([]*ParsedFile{f}); e != nil {
		t.Fatal(e)
	}
	return c
}
func TestNativeIRReports(t *testing.T) {
	for i := range 8 {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			t.Parallel()
			c := reportSourceContext(t)
			g := c.Typecheck.IR
			fn := driverFunc(t, c, "F")
			reports := bindIRReports(t, c)
			var trace bytes.Buffer
			g.TraceWriter = &trace
			g.AstDump(fn, "start, F")
			g.AstDump(fn, "unchanged, F")
			c.Interleaved.DevirtualizeAndInlinePackage(c.Typecheck.Target, nil)
			g.AstDump(fn, "after-inline, F")
			g.CloseHTMLWriters()
			if len(reports) != 2 {
				t.Fatal("wrong report count", len(reports))
			}
			for path, f := range reports {
				if f.opens != f.closes {
					t.Fatal("unclosed report", path, f.opens, f.closes)
				}
				if strings.HasSuffix(path, ".ast") {
					if f.opens != 3 || !strings.Contains(f.String(), "start, F") || !strings.Contains(f.String(), "unchanged, F") || !strings.Contains(f.String(), "after-inline, F") {
						t.Fatal("append report lost phase")
					}
				} else if f.opens != 1 || !strings.Contains(f.String(), "start") || !strings.Contains(f.String(), "unchanged") || !strings.Contains(f.String(), "after-inline") || !strings.Contains(f.String(), "INLCALL") || !strings.HasSuffix(f.String(), "</html>\n") {
					t.Fatal("incomplete HTML report")
				}
			}
			g.CloseHTMLWriters() // native registry is drained
			for _, f := range reports {
				if f.closes != f.opens {
					t.Fatal("duplicate close")
				}
			}
			if !strings.Contains(trace.String(), "Writing text ast output") || !strings.Contains(trace.String(), "Writing html ast output") {
				t.Fatal("missing report trace")
			}
			// Generic native dumping uses the same position table and hides ownership.
			var any bytes.Buffer
			g.FDumpAny(&any, fn, "Body|Results|X|Y|Fun|Args", 7)
			if !strings.Contains(any.String(), "ir.Nodes") || strings.Contains(any.String(), "compilation") {
				t.Fatal("generic dump missing graph or leaking owner", any.String())
			}
			// Closing HTML does not reset the native text append history.
			g.AstDump(fn, "later, F")
			g.CloseHTMLWriters()
			for p, f := range reports {
				if strings.HasSuffix(p, ".ast") && (f.opens != 4 || !strings.Contains(f.String(), "start, F") || !strings.Contains(f.String(), "later, F")) {
					t.Fatal("native append history reset")
				}
			}
		})
	}
}
func TestIRReportBoundaries(t *testing.T) {
	c := reportSourceContext(t)
	g := c.Typecheck.IR
	fn := driverFunc(t, c, "F")
	other := reportSourceContext(t)
	foreign := driverFunc(t, other, "F")
	opens := 0
	g.OpenAST = func(string, int, fs.FileMode) (io.WriteCloser, error) { opens++; return nil, errors.New("open") }
	mustPanicPrepackage(t, func() { g.AstDump(foreign, "bad") })
	mustPanicPrepackage(t, func() { g.NewHTMLWriter("bad", foreign, "") })
	mustPanicPrepackage(t, func() { g.DumpNodeHTML(fn, "bad", foreign) })
	mustPanicPrepackage(t, func() { g.FDumpAny(io.Discard, foreign, "", 2) })
	if opens != 0 {
		t.Fatal("foreign graph opened reports")
	}
	mustPanicPrepackage(t, func() { g.AstDump(fn, "open-failure") })
	g.OpenAST = nil
	mustPanicPrepackage(t, func() { g.NewHTMLWriter("nil", fn, "") })
	g.OpenAST = func(string, int, fs.FileMode) (io.WriteCloser, error) { return nil, nil }
	mustPanicPrepackage(t, func() { g.NewHTMLWriter("nil-result", fn, "") })
}
func TestIRTextFailureStillWritesHTML(t *testing.T) {
	for _, failure := range []string{"open", "close"} {
		t.Run(failure, func(t *testing.T) {
			c := reportSourceContext(t)
			g := c.Typecheck.IR
			fn := driverFunc(t, c, "F")
			reports := bindIRReports(t, c)
			open := g.OpenAST
			sentinel := errors.New("injected text " + failure)
			var trace bytes.Buffer
			g.TraceWriter = &trace
			g.OpenAST = func(p string, f int, m fs.FileMode) (io.WriteCloser, error) {
				if strings.HasSuffix(p, ".ast") && failure == "open" {
					return nil, sentinel
				}
				w, e := open(p, f, m)
				if strings.HasSuffix(p, ".ast") && e == nil {
					w.(*irReportHandle).closeError = sentinel
				}
				return w, e
			}
			g.AstDump(fn, "start, F")
			g.CloseHTMLWriters()
			if !strings.Contains(trace.String(), "Dump returned error "+sentinel.Error()) {
				t.Fatal("lost native text error", trace.String())
			}
			found := false
			for p, f := range reports {
				if strings.HasSuffix(p, ".html") {
					found = true
					if f.closes != 1 || !strings.HasSuffix(f.String(), "</html>\n") {
						t.Fatal("HTML did not finish")
					}
				}
			}
			if !found {
				t.Fatal("text error prevented HTML")
			}
		})
	}
}

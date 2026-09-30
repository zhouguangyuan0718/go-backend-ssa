package ssa

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestNativeHTMLConstructorAndMasks(t *testing.T) {
	for i := range 8 {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			t.Parallel()
			f, _ := diagnosticFixture(8)
			c := diagnosticContext(f)
			c.initializePhases()
			var trace bytes.Buffer
			c.TraceWriter = &trace
			var out diagnosticBuffer
			var calls []string
			c.OpenHTML = func(path string, flags int, mode fs.FileMode) (io.WriteCloser, error) {
				calls = append(calls, "open")
				if path != "reports/a.html" || flags != os.O_WRONLY|os.O_CREATE|os.O_TRUNC || mode != 0644 {
					t.Error(path, flags, mode)
				}
				return &out, nil
			}
			c.WorkingDirectory = func() (string, error) { calls = append(calls, "cwd"); return fmt.Sprint("/work/", i), nil }
			c.LookPath = func(name string) (string, error) {
				calls = append(calls, "lookup")
				if name != "dot" {
					t.Error(name)
				}
				return "controlled/dot", nil
			}
			w := NewHTMLWriter("reports/a.html", f, "")
			if !slices.Equal(calls, []string{"open", "cwd"}) || w.path != filepath.Join(fmt.Sprint("/work/", i), "reports/a.html") || w.dot != nil || w.Func != f {
				t.Fatal("native constructor", calls, w)
			}
			w.WriteSources("sources", []*FuncLines{{Filename: "source.go", StartLineno: 2, Lines: []string{"a < b && c", ""}}})
			w.Close()
			if !out.closed || !strings.HasPrefix(out.String(), "<html>") || !strings.HasSuffix(out.String(), "</html>") || !strings.Contains(out.String(), "a &lt; b &amp;&amp; c") || !strings.Contains(trace.String(), w.path) {
				t.Error("HTML lifecycle")
			}
			// Native phase masks include exact ranges, unions, underscore normalization,
			// all phases, and invalid/reversed ranges. Invalid masks never look up dot.
			first, last := c.passes[0].name, c.passes[len(c.passes)-1].name
			for _, tt := range []struct {
				mask string
				want []string
			}{
				{"", nil}, {"*", func() []string {
					var x []string
					for _, p := range c.passes {
						x = append(x, p.name)
					}
					return x
				}()},
				{strings.ReplaceAll(first, " ", "_"), []string{first}},
				{first + "," + last, []string{first, last}},
				{first + "-" + c.passes[2].name, []string{first, c.passes[1].name, c.passes[2].name}},
				{last + "-" + first, nil}, {"bad-phase-range", nil}, {"unknown", nil},
			} {
				before := len(calls)
				d := c.newDotWriter(tt.mask)
				if tt.want == nil {
					if d != nil || len(calls) != before {
						t.Error("invalid/empty mask", tt.mask)
					}
					continue
				}
				if d == nil || d.path != "controlled/dot" || len(d.phases) != len(tt.want) || len(calls) != before+1 {
					t.Fatal(tt.mask, d, calls)
				}
				for _, name := range tt.want {
					if !d.phases[name] {
						t.Error("missing phase", name)
					}
				}
			}
			failure := errors.New("dot unavailable")
			c.LookPath = func(string) (string, error) { return "", failure }
			if c.newDotWriter(first) != nil || !strings.Contains(trace.String(), failure.Error()) {
				t.Error("lookup failure")
			}
			// Absolute report paths skip cwd. Selected graph phases do query dot.
			calls = nil
			out = diagnosticBuffer{}
			c.OpenHTML = func(string, int, fs.FileMode) (io.WriteCloser, error) {
				calls = append(calls, "open")
				return &out, nil
			}
			c.LookPath = func(string) (string, error) { calls = append(calls, "lookup"); return "controlled/dot", nil }
			w = NewHTMLWriter("/reports/a.html", f, first)
			w.Close()
			if !slices.Equal(calls, []string{"open", "lookup"}) || w.path != "/reports/a.html" {
				t.Error(calls, w.path)
			}
		})
	}
}

func TestNativeHTMLConstructorFailures(t *testing.T) {
	for _, stage := range []string{"open", "cwd"} {
		t.Run(stage, func(t *testing.T) {
			f, fe := diagnosticFixture(8)
			c := diagnosticContext(f)
			var out diagnosticBuffer
			failure := errors.New("injected " + stage)
			c.OpenHTML = func(string, int, fs.FileMode) (io.WriteCloser, error) {
				if stage == "open" {
					return nil, failure
				}
				return &out, nil
			}
			c.WorkingDirectory = func() (string, error) {
				if stage != "cwd" {
					t.Fatal("cwd after open failure")
				}
				return "", failure
			}
			defer func() {
				if got := recover(); got != failure.Error() || fe.fatalCount != 1 || out.Len() != 0 {
					t.Error(got, fe.fatalCount, out.Len())
				}
			}()
			NewHTMLWriter("report.html", f, "")
			t.Fatal("missing fatal")
		})
	}
}

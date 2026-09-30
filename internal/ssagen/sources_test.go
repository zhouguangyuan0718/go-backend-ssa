package ssagen

import (
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"testing"
)

type sourceReader struct {
	io.Reader
	closes int
}

func (r *sourceReader) Close() error { r.closes++; return errors.New("native ignores close failure") }

type failedSourceReader struct{}

func (failedSourceReader) Read([]byte) (int, error) { return 0, errors.New("native scanner failure") }

func TestNativeSourceLinesAndDumpEnvironment(t *testing.T) {
	for i := range 8 {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			t.Parallel()
			c := frontendContext("amd64", false)
			c.DumpFunction = fmt.Sprint("f", i, ":early_phielim-late_opt+")
			c.DumpDirectory = fmt.Sprint("reports/", i)
			c.InitEnv()
			if c.ssaDump != fmt.Sprint("f", i) || c.ssaDir != c.DumpDirectory || !c.ssaDumpStdout || c.ssaDumpCFG != "early_phielim-late_opt" {
				t.Fatal("environment parsing")
			}
			fn := frontendFunction(c, fmt.Sprint("f", i))
			c.DumpInline(fn)
			c.DumpInline(frontendFunction(c, "other"))
			if len(c.ssaDumpInlined) != 1 || c.ssaDumpInlined[0] != fn {
				t.Fatal("inline selection")
			}
			for _, tt := range []struct {
				start, end uint
				want       []string
			}{{2, 3, []string{"two <&>", "三"}}, {1, 9, []string{"one", "two <&>", "三", "last"}}, {8, 9, nil}, {3, 2, nil}, {0, 1, []string{"one"}}} {
				r := &sourceReader{Reader: strings.NewReader("one\ntwo <&>\n三\nlast")}
				var calls []string
				c.ExpandSourcePath = func(path string) string {
					calls = append(calls, "expand")
					return strings.ReplaceAll(path, "$SRC", fmt.Sprint(i))
				}
				c.OpenSource = func(path string) (io.ReadCloser, error) {
					calls = append(calls, "open")
					if path != fmt.Sprint(i, "/input.go") {
						t.Error(path)
					}
					return r, nil
				}
				lines, err := c.readFuncLines("$SRC/input.go", tt.start, tt.end)
				if err != nil || lines.Filename != "$SRC/input.go" || lines.StartLineno != tt.start || !slices.Equal(lines.Lines, tt.want) || r.closes != 1 || !slices.Equal(calls, []string{"expand", "open"}) {
					t.Error(lines, err, r.closes, calls)
				}
			}
			failure := errors.New("open failed")
			c.OpenSource = func(string) (io.ReadCloser, error) { return nil, failure }
			if lines, err := c.readFuncLines("x", 1, 2); lines != nil || err != failure {
				t.Error(lines, err)
			}
			// Preserve the source algorithm's treatment of scanner/close errors, including
			// an overlong line, rather than adding behavior that upstream does not have.
			for _, reader := range []io.Reader{failedSourceReader{}, strings.NewReader(strings.Repeat("x", 70000))} {
				r := &sourceReader{Reader: reader}
				c.OpenSource = func(string) (io.ReadCloser, error) { return r, nil }
				lines, err := c.readFuncLines("x", 1, 2)
				if err != nil || len(lines.Lines) != 0 || r.closes != 1 {
					t.Error(lines, err, r.closes)
				}
			}
		})
	}
	for _, spec := range []struct {
		input, name, cfg string
		stdout           bool
	}{{"", "", "", false}, {"f", "f", "", false}, {"f+", "f", "", true}, {"f:a:b", "f", "a", false}, {"f:a+", "f", "a", true}} {
		c := frontendContext("amd64", false)
		c.DumpFunction = spec.input
		c.InitEnv()
		if c.ssaDump != spec.name || c.ssaDumpCFG != spec.cfg || c.ssaDumpStdout != spec.stdout {
			t.Error(spec, c.ssaDump, c.ssaDumpCFG, c.ssaDumpStdout)
		}
	}
}

func TestNativeSourceRequiresProvidersAndLocalInline(t *testing.T) {
	c := frontendContext("amd64", false)
	for _, f := range []func(){func() { c.readFuncLines("existing.go", 1, 2) }, func() { c.ExpandSourcePath = func(s string) string { return s }; c.readFuncLines("existing.go", 1, 2) }} {
		func() {
			defer func() {
				if recover() == nil {
					t.Error("missing provider accepted")
				}
			}()
			f()
		}()
	}
	c.DumpFunction = "f"
	c.InitEnv()
	local := frontendFunction(c, "f")
	c.DumpInline(local)
	foreign := frontendFunction(frontendContext("amd64", false), "f")
	func() {
		defer func() {
			if recover() == nil {
				t.Error("foreign inline accepted")
			}
		}()
		c.DumpInline(foreign)
	}()
	if len(c.ssaDumpInlined) != 1 || c.ssaDumpInlined[0] != local {
		t.Error("inline list mutated before rejection")
	}
}

package ssa

import (
	"bytes"
	"fmt"
	"html"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"

	"go-backend-ssa/internal/cmd/src"
)

type diagnosticFrontend struct {
	Frontend
	enabled       bool
	log           bytes.Buffer
	fatalPosition src.XPos
	fatalMessage  string
	fatalCount    int
}

func (f *diagnosticFrontend) Log() bool                       { return f.enabled }
func (f *diagnosticFrontend) Logf(format string, args ...any) { fmt.Fprintf(&f.log, format, args...) }
func (f *diagnosticFrontend) Fatalf(pos src.XPos, format string, args ...any) {
	f.fatalPosition = pos
	f.fatalMessage = fmt.Sprintf(format, args...)
	f.fatalCount++
	panic(f.fatalMessage)
}

type diagnosticBuffer struct {
	bytes.Buffer
	closed bool
}

func (b *diagnosticBuffer) Close() error { b.closed = true; return nil }

func diagnosticFixture(size int) (*Func, *diagnosticFrontend) {
	u, abi := auxFixture(size)
	cfg := &Config{PtrSize: int64(size), RegSize: int64(size), Types: *NewTypes(u), ctxt: u.LinkContext().Ctxt}
	fe := &diagnosticFrontend{}
	cache := new(Cache)
	cache.Reset()
	f := cfg.NewFunc(fe, cache)
	f.Name = "print<&>"
	f.Type = u.NewSignature(nil, nil, nil)
	f.ABISelf = abi
	f.pass = &pass{name: "diagnostic phase"}
	f.Entry = f.NewBlock(BlockRet)
	f.Entry.Pos = cfg.ctxt.PosTable.XPos(src.MakePos(src.NewFileBase("diagnostic.go", "diagnostic.go"), 17, 3))
	v := f.Entry.NewValue0I(f.Entry.Pos, OpConst64, cfg.Types.Int64, 42)
	f.Entry.SetControl(v)
	return f, fe
}

func TestNativeDiagnosticOrderingAndIsolation(t *testing.T) {
	retained, _ := diagnosticFixture(4)
	before := retained.String()
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Go(func() {
			f, fe := diagnosticFixture(8)
			fe.enabled = true
			var output diagnosticBuffer
			f.HTMLWriter = &HTMLWriter{w: &output, Func: f}
			func() {
				defer func() {
					if got := recover(); got != "broken 9" {
						t.Errorf("fatal result: %v", got)
					}
				}()
				f.Entry.Fatalf("broken %d", 9)
			}()
			if fe.fatalCount != 1 || fe.fatalPosition != f.Entry.Pos || !strings.Contains(fe.log.String(), "pass diagnostic phase end crashed") || !strings.Contains(fe.log.String(), "Const64") || !strings.Contains(output.String(), "crashed") || len(f.HTMLWriter.pendingPhases) != 0 {
				t.Error("native crash dump or fatal forwarding", fe.log.String(), output.String())
			}
			oldHTML, oldLog := output.String(), fe.log.String()
			v := f.Entry.Values[0]
			func() {
				defer func() {
					if recover() == nil {
						t.Error("Aux type check did not fail")
					}
				}()
				v.AuxInt8()
			}()
			if fe.fatalCount != 2 || fe.fatalPosition != v.Pos || output.String() != oldHTML || fe.log.String() != oldLog {
				t.Error("Value.Fatalf must retain direct frontend reporting")
			}
			var trace bytes.Buffer
			f.Config.Types.compilation.TraceWriter = &trace
			f.HTMLWriter.Close()
			if !output.closed || !strings.HasSuffix(output.String(), "</html>") || !strings.Contains(trace.String(), "dumped SSA for print<&>,1") {
				t.Error("native HTML close")
			}
			if retained.Entry.Values[0].AuxInt != 42 || retained.Config.PtrSize != 4 {
				t.Error("retained graph changed")
			}
		})
	}
	wg.Wait()
	if retained.String() != before {
		t.Fatal("retained diagnostic output changed")
	}
	var absent *HTMLWriter
	absent.Close()
	absent.WritePhase("ignored", "ignored")
	absent.WriteSources("ignored", nil)
	absent.WriteAST("ignored", nil)
	if diagnosticContext(retained).opcodeTables != nil {
		t.Fatal("diagnostics allocated opcode tables")
	}
}

// This subprocess stands in for the renderer protocol, exercising real pipes,
// process start/wait and error branches without requiring Graphviz installation.
func TestSSADiagnosticRendererHelper(t *testing.T) {
	mode := os.Getenv("SYNCGO_SSA_RENDERER_HELPER")
	if mode == "" {
		return
	}
	input, err := io.ReadAll(os.Stdin)
	if err != nil {
		os.Exit(9)
	}
	switch mode {
	case "fail":
		fmt.Fprint(os.Stderr, "renderer failed")
		os.Exit(7)
	case "malformed":
		fmt.Print("no SVG element")
	default:
		fmt.Printf("<svg xmlns=\"http://www.w3.org/2000/svg\"><desc>%s</desc></svg>", html.EscapeString(string(input)))
	}
	os.Exit(0)
}

func TestNativeSVGResourceBinding(t *testing.T) {
	for _, mode := range []string{"valid", "pipe-error", "start-error", "fail", "malformed"} {
		t.Run(mode, func(t *testing.T) {
			f, _ := diagnosticFixture(8)
			tail := f.NewBlock(BlockPlain)
			f.Entry.AddEdgeTo(tail)
			tail.AddEdgeTo(f.Entry)
			f.laidout = true
			f.Entry.Likely = BranchUnlikely
			var trace, output bytes.Buffer
			calls := 0
			c := f.Config.Types.compilation
			c.TraceWriter = &trace
			c.Command = func(name string, args ...string) *exec.Cmd {
				calls++
				if name != "reviewed-dot" || len(args) != 1 || args[0] != "-Tsvg" {
					t.Error("native renderer command changed", name, args)
				}
				cmd := exec.Command(os.Args[0], "-test.run=^TestSSADiagnosticRendererHelper$")
				cmd.Env = append(os.Environ(), "SYNCGO_SSA_RENDERER_HELPER="+mode)
				if mode == "pipe-error" {
					cmd.Stdin = strings.NewReader("")
				}
				if mode == "start-error" {
					cmd.Path = t.TempDir() + "/missing-renderer"
				}
				return cmd
			}
			dot := &dotWriter{path: "reviewed-dot", phases: map[string]bool{"diagnostic phase": true}}
			dot.writeFuncSVG(&output, "disabled", f)
			if calls != 0 {
				t.Fatal("disabled phase started command")
			}
			dot.writeFuncSVG(&output, "diagnostic phase", f)
			if calls != 1 {
				t.Fatal("command count", calls)
			}
			switch mode {
			case "valid":
				if dot.broken || trace.Len() != 0 || !strings.Contains(output.String(), `id="svg_graph_diagnostic-phase" onload="makeDraggable(evt)"`) || !strings.Contains(output.String(), "ranksep=.2") || !strings.Contains(output.String(), "#2893ff") {
					t.Fatal("native SVG or DOT output", output.String(), trace.String())
				}
			case "pipe-error", "start-error", "fail":
				if !dot.broken || trace.Len() == 0 {
					t.Fatal("renderer failure not retained", trace.String())
				}
				dot.writeFuncSVG(&output, "diagnostic phase", f)
				if calls != 1 {
					t.Fatal("broken renderer restarted")
				}
			case "malformed":
				if dot.broken || !strings.Contains(trace.String(), "couldn't find dot sep") {
					t.Fatal("native malformed SVG handling", trace.String())
				}
			}
		})
	}
	t.Run("missing provider", func(t *testing.T) {
		f, _ := diagnosticFixture(8)
		dot := &dotWriter{phases: map[string]bool{"enabled": true}}
		defer func() {
			if got := recover(); got != "SSA graph rendering requires a command provider" {
				t.Fatalf("missing provider: %v", got)
			}
		}()
		dot.writeFuncSVG(io.Discard, "enabled", f)
	})

	// Native early returns do not dereference a missing function.
	var output bytes.Buffer
	(&dotWriter{broken: true}).writeFuncSVG(&output, "ignored", nil)
	(&dotWriter{}).writeFuncSVG(&output, "ignored", nil)
}

type diagnosticFailWriter struct{}

func (diagnosticFailWriter) Write([]byte) (int, error) { return 0, fmt.Errorf("output failed") }
func (diagnosticFailWriter) Close() error              { return nil }
func TestNativeHTMLWriteFailure(t *testing.T) {
	for _, formatted := range []bool{false, true} {
		f, fe := diagnosticFixture(8)
		w := &HTMLWriter{w: diagnosticFailWriter{}, Func: f}
		func() {
			defer func() {
				if got := recover(); got != "output failed" {
					t.Errorf("write failure: %v", got)
				}
			}()
			if formatted {
				w.Printf("%s", "text")
			} else {
				w.WriteString("text")
			}
		}()
		if fe.fatalCount != 1 || fe.fatalPosition != (src.XPos{}) {
			t.Fatal("HTML output failure position")
		}
	}
}

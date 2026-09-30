package ssa

import (
	"bytes"
	"errors"
	"fmt"
	"go-backend-ssa/internal/cmd/src"
	"io"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestNativeCompiledPhysicalValues(t *testing.T) {
	for _, seed := range []int{0, 1, 314159} {
		for _, optimize := range []bool{false, true} {
			t.Run(fmt.Sprint(seed, "/", optimize), func(t *testing.T) {
				testAllocatedPhysicalValues(t, func(f *Func) {
					c := diagnosticContext(f)
					c.Now = func() time.Time { return time.Time{} }
					f.Config.optimize = optimize
					if !optimize {
						c.IR.Base.Flag.N = 1
					}
					if seed != 0 {
						if err := c.PhaseOption("check", "seed", seed, ""); err != "" {
							t.Fatal(err)
						}
					}
					Compile(f)
					checkFunc(f)
				})
			})
		}
	}
}

type compileFrontend struct {
	*callsFrontend
	warnings []string
}

func (fe *compileFrontend) Warnl(p src.XPos, format string, args ...any) {
	fe.warnings = append(fe.warnings, fmt.Sprintf(format, args...))
}

type compileBuffer struct {
	bytes.Buffer
	closed bool
}

func (b *compileBuffer) Close() error {
	if b.closed {
		panic("closed twice")
	}
	b.closed = true
	return nil
}

func TestNativeCompileDiagnosticsAndProviders(t *testing.T) {
	c := allocationContext("amd64")
	f := allocationPressureGraph(c)
	f.Name = "example /:fn"
	fe := &compileFrontend{callsFrontend: f.fe.(*callsFrontend)}
	fe.enabled = true
	f.fe = fe
	var trace bytes.Buffer
	c.TraceWriter = &trace
	f.ruleMatches = map[string]int{"z": 2, "a": 1}
	if err := c.PhaseOption("all", "dump", 1, f.Name); err != "" {
		t.Fatal(err)
	}
	// Native all/time and all/mem replace each other's booleans. Set both on a
	// specific phase to exercise its combined statistics without changing policy.
	c.PhaseOption("number_lines", "time", 1, "")
	c.PhaseOption("number_lines", "mem", 1, "")
	c.DumpDirectory = filepath.Join("virtual", "ssa")
	outputs := map[string]*compileBuffer{}
	c.CreateDump = func(name string) (io.WriteCloser, error) {
		if _, exists := outputs[name]; exists {
			t.Fatal("dump name reused")
		}
		b := new(compileBuffer)
		outputs[name] = b
		return b, nil
	}
	clock, metrics := 0, 0
	var observed []string
	c.Now = func() time.Time {
		clock++
		observed = append(observed, f.pass.name)
		return time.Unix(0, int64(clock*17))
	}
	c.ReadMemStats = func(m *runtime.MemStats) {
		metrics++
		m.TotalAlloc = uint64(metrics * 64)
		m.Mallocs = uint64(metrics * 3)
	}
	Compile(f)
	var names []string
	for _, p := range c.passes {
		if !p.disabled {
			names = append(names, p.name)
		}
	}
	if clock != len(names)*2 || metrics != 2 || len(outputs) != len(names)+1 {
		t.Fatalf("clock=%d metrics=%d outputs=%d stages=%d", clock, metrics, len(outputs), len(names))
	}
	for i, name := range names {
		if observed[2*i] != name || observed[2*i+1] != name {
			t.Fatalf("phase order %v", observed)
		}
	}
	dumpNames := append([]string{"build"}, names...)
	for i, phase := range dumpNames {
		name := filepath.Join(c.DumpDirectory, fmt.Sprintf("example___fn_%02d__%s.dump", i+1, strings.ReplaceAll(phase, " ", "_")))
		b := outputs[name]
		if b == nil || !b.closed || !strings.Contains(b.String(), f.Name) {
			t.Fatalf("missing/unclosed native dump %s", name)
		}
	}
	warnings := strings.Join(fe.warnings, "\n")
	if !strings.Contains(warnings, "TIME(ns)\t17") || !strings.Contains(warnings, "TIME(ns):BYTES:ALLOCS\t17\t64\t3") {
		t.Fatal(warnings)
	}
	if !strings.Contains(fe.log.String(), "pass number lines begin") || !strings.Contains(fe.log.String(), "pass trim end [17 ns]") {
		t.Fatal("native phase progress missing")
	}
	if trace.String() != f.Name+": a=1 z=2 \n" {
		t.Fatalf("rule totals %q", trace.String())
	}
	if !f.scheduled || f.RegAlloc == nil {
		t.Fatal("pipeline did not allocate")
	}
}

func TestNativeCompileDumpFailureAndPanicPhase(t *testing.T) {
	c := allocationContext("amd64")
	f := allocationPressureGraph(c)
	fe := &compileFrontend{callsFrontend: f.fe.(*callsFrontend)}
	f.fe = fe
	c.Now = func() time.Time { return time.Time{} }
	c.PhaseOption("build", "dump", 1, f.Name)
	attempts := 0
	c.CreateDump = func(string) (io.WriteCloser, error) { attempts++; return nil, errors.New("output unavailable") }
	Compile(f)
	if attempts != 1 || len(fe.warnings) != 1 || !strings.Contains(fe.warnings[0], "Unable to create after-phase dump file fixture_01__build.dump") || f.RegAlloc == nil {
		t.Fatalf("native failed-dump behavior: %v", fe.warnings)
	}
	for _, kind := range []string{"clock", "metrics", "dump", "stage"} {
		t.Run(kind, func(t *testing.T) {
			c := allocationContext("amd64")
			f := allocationPressureGraph(c)
			c.Now = func() time.Time { return time.Time{} }
			phase, want := "number lines", "clock provider"
			switch kind {
			case "clock":
				c.Now = nil
			case "metrics":
				c.PhaseOption("number_lines", "mem", 1, "")
				want = "metric provider"
			case "dump":
				c.PhaseOption("build", "dump", 1, f.Name)
				phase = "init"
				want = "output provider"
			case "stage":
				i := phaseIndex(c, "opt")
				c.passes[i].fn = func(*Func) { panic("deliberate-stage-panic") }
				phase = "opt"
				want = "deliberate-stage-panic"
			}
			defer func() {
				value := recover()
				message := fmt.Sprint(value)
				if value == nil || !strings.Contains(message, "panic during "+phase+" while compiling fixture") || !strings.Contains(message, want) || !strings.Contains(message, "goroutine ") {
					t.Fatalf("phase diagnostic: %s", message)
				}
			}()
			Compile(f)
			t.Fatal("failing compilation returned")
		})
	}
}

func TestNativeCompileParallelContexts(t *testing.T) {
	var wg sync.WaitGroup
	for i := range 8 {
		wg.Go(func() {
			c := allocationContext([]string{"amd64", "arm64", "riscv64", "wasm"}[i%4])
			f := allocationPressureGraph(c)
			f.Name = fmt.Sprint("parallel-", i)
			c.PhaseOption("check", "seed", i+1, "")
			c.PhaseOption("build", "dump", 1, f.Name)
			c.Now = func() time.Time { return time.Time{} }
			var b compileBuffer
			c.CreateDump = func(name string) (io.WriteCloser, error) {
				if !strings.HasPrefix(name, f.Name+"_01__") {
					t.Error(name)
				}
				return &b, nil
			}
			Compile(f)
			checkFunc(f)
			if !b.closed || !strings.Contains(b.String(), f.Name) || !f.scheduled || f.RegAlloc == nil {
				t.Error("parallel compilation lost output/state")
			}
		})
	}
	wg.Wait()
}

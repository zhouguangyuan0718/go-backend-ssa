package ssa

import (
	"bytes"
	"fmt"
	"strings"
	"sync"
	"testing"
)

func phaseIndex(c *Context, name string) int {
	for i, p := range c.passes {
		if p.name == name {
			return i
		}
	}
	panic("missing phase " + name)
}

func TestNativePhasePolicyAndOptions(t *testing.T) {
	for _, simd := range []bool{false, true} {
		for _, loops := range []bool{false, true} {
			t.Run(fmt.Sprint(simd, "/", loops), func(t *testing.T) {
				base := loweringContext("amd64")
				base.Types.Experiment.SIMD = simd
				base.Types.Experiment.PreemptibleLoops = loops
				c := NewContext(base.ABI, base.ReflectData, base.Log)
				feature := c.passes[phaseIndex(c, "cpufeatures")]
				tern := c.passes[phaseIndex(c, "rewrite tern")]
				resched := c.passes[phaseIndex(c, "insert resched checks")]
				if feature.required != simd || feature.disabled == simd || tern.disabled == simd || tern.required || resched.disabled == loops {
					t.Fatal("experiment policy lost")
				}
				names := map[string]int{}
				for i, p := range c.passes {
					if p.name == "" || p.fn == nil {
						t.Fatal("missing native phase")
					}
					if _, ok := names[p.name]; ok {
						t.Fatal("duplicate phase")
					}
					names[p.name] = i
				}
				for _, order := range c.passOrder {
					a, aok := names[order.a]
					b, bok := names[order.b]
					if !aok || !bok || a >= b {
						t.Fatalf("invalid phase order %+v", order)
					}
				}
				option := func(phase, flag string, value int, arg string) {
					t.Helper()
					if err := c.PhaseOption(phase, flag, value, arg); err != "" {
						t.Fatal(err)
					}
				}
				option("check", "seed", 1234, "")
				if !c.checkEnabled || !c.debugPoset || c.checkRandSeed != 1234 {
					t.Fatal("checker seed lost")
				}
				option("check", "off", 1, "")
				if c.checkEnabled || c.debugPoset {
					t.Fatal("checker off lost")
				}
				option("all", "dump", 1, "example.fn")
				if !c.BuildDump["example.fn"] || !c.GenssaDump["example.fn"] {
					t.Fatal("build/genssa dump not set")
				}
				for _, p := range c.passes {
					if !p.dump["example.fn"] {
						t.Fatal("phase dump not set")
					}
				}
				option("~.*cse$", "debug", 7, "")
				for _, p := range c.passes {
					if strings.HasSuffix(p.name, "cse") && p.debug != 7 {
						t.Fatal("regexp options lost")
					}
				}
				before := c.passes[phaseIndex(c, "regalloc")]
				if err := c.PhaseOption("regalloc", "off", 1, ""); !strings.Contains(err, "Cannot disable required") {
					t.Fatal(err)
				}
				after := c.passes[phaseIndex(c, "regalloc")]
				if after.disabled != before.disabled {
					t.Fatal("failed required phase disable changed state")
				}
				option("generic_cse", "off", 1, "")
				if !c.passes[phaseIndex(c, "generic cse")].disabled {
					t.Fatal("underscore phase spelling lost")
				}
				option("intrinsics", "off", 1, "")
				option("intrinsics", "debug", 4, "")
				option("build", "debug", 2, "")
				option("build", "test", 3, "")
				option("build", "stats", 4, "")
				if !c.IntrinsicsDisable || c.IntrinsicsDebug != 4 || c.BuildDebug != 2 || c.BuildTest != 3 || c.BuildStats != 4 {
					t.Fatal("frontend policy flags lost")
				}
				if c.PhaseOption("~[", "debug", 1, "") == "" || c.PhaseOption("unknown", "on", 1, "") == "" {
					t.Fatal("invalid option accepted")
				}
				if base.BuildDump["example.fn"] || base.GenssaDump["example.fn"] || base.IntrinsicsDisable {
					t.Fatal("shared phase state")
				}
			})
		}
	}
}

func TestNativePhaseContextIsolation(t *testing.T) {
	old := loweringContext("386")
	old.PhaseOption("all", "dump", 1, "retained")
	var wg sync.WaitGroup
	for i := range 8 {
		wg.Go(func() {
			c := loweringContext("amd64")
			label := fmt.Sprint(i)
			c.PhaseOption("check", "seed", i+1, "")
			c.PhaseOption("all", "dump", 1, label)
			c.PhaseOption("prove", "debug", i+2, "")
			if c.checkRandSeed != i+1 || c.passes[phaseIndex(c, "prove")].debug != i+2 || len(c.BuildDump) != 1 || c.BuildDump["retained"] {
				t.Error("phase state escaped its context")
			}
			for _, p := range c.passes {
				if len(p.dump) != 1 || !p.dump[label] {
					t.Error("shared phase dump map")
				}
			}
		})
	}
	wg.Wait()
	if old.checkEnabled || len(old.BuildDump) != 1 || !old.BuildDump["retained"] || old.passes[phaseIndex(old, "prove")].debug != 0 {
		t.Fatal("retained context changed")
	}
	metadata := newOpcodeContext()
	if metadata.Types != nil || metadata.passes[0].fn != nil || metadata.BuildDump != nil || metadata.opcodeTable[OpAdd64].name != "Add64" {
		t.Fatal("metadata initialized compilation policy")
	}
}

func TestNativePhasePanicDiagnostics(t *testing.T) {
	c := loweringContext("amd64")
	var output bytes.Buffer
	c.TraceWriter = &output
	defer func() {
		if got := recover(); got != "pass missing not found" || output.String() != "pass missing not found\n" {
			t.Fatalf("panic=%v output=%q", got, output.String())
		}
	}()
	c.panicLogf("pass %s not found", "missing")
	t.Fatal("diagnostic returned")
}

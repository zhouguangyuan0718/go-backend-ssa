package base

import (
	"bytes"
	"go-backend-ssa/internal/cmd/obj"
	"go-backend-ssa/internal/cmd/src"
	"testing"
)

func TestNativeDebugFlagPolicies(t *testing.T) {
	var output bytes.Buffer
	c := NewLinkContext(&obj.Link{Pkgpath: "p", TraceWriter: &output})
	c.Flag.LowerC = 4
	c.Flag.N = 1
	c.Flag.S = 3
	c.Flag.Race = true
	d := DefaultDebugFlags()
	d.Gossahash = "qn"
	d.LoopVarHash = "ILqn"
	d.Converthash = "qn"
	d.Fmahash = "qn"
	d.PGOHash = "qn"
	d.LiteralAllocHash = "qn"
	d.MergeLocalsHash = "qn"
	d.VariableMakeHash = "qn"
	d.PCTab = "pctospadj"
	d.MayMoreStack = "p.morestack"
	d.NoRefName = 1
	d.CompressInstructions = 0
	if err := c.PrepareNativeFlags(&d); err != nil {
		t.Fatal(err)
	}
	for _, h := range []*HashDebug{c.hashDebug, c.LoopVarHash, c.ConvertHash, c.FmaHash, c.PGOHash, c.LiteralAllocHash, c.MergeLocalsHash, c.VariableMakeHash} {
		if h == nil || h.compilation != c || h.logfile != &output || h.MatchPos(src.XPos{}, nil) {
			t.Fatal("hash binding", h)
		}
	}
	if d.LoopVarHash != "qn" || d.LoopVar != 1 || c.LoopVarHash.inlineSuffixOnly || d.Checkptr != 1 || c.Flag.LowerC != 1 {
		t.Fatal("native flag normalization", d, c.Flag.LowerC)
	}
	if c.Ctxt.CompressInstructions || c.Ctxt.Flag_optimize || c.Ctxt.Debugasm != 3 || c.Ctxt.Flag_maymorestack != "p.morestack" || !c.Ctxt.Flag_noRefName || c.Ctxt.Debugpcln != "pctospadj" {
		t.Fatal("native object flag binding")
	}
	if output.Len() != 0 {
		t.Fatal("quiet patterns emitted output", output.String())
	}
	other := NewLinkContext(&obj.Link{Pkgpath: "p"})
	other.Flag.LowerC = 4
	plain := DefaultDebugFlags()
	if err := other.PrepareNativeFlags(&plain); err != nil {
		t.Fatal(err)
	}
	if other.HasDebugHash() || other.LoopVarHash != nil || other.FmaHash != nil || other.PGOHash != nil || other.ConvertHash == nil || other.ConvertHash == c.ConvertHash || plain.Checkptr != 0 || other.Flag.LowerC != 4 || !other.Ctxt.Flag_optimize || !other.Ctxt.CompressInstructions {
		t.Fatal("state leaked into default compilation")
	}
}

func TestNativeLoopVarAndRuntimeFlags(t *testing.T) {
	for _, value := range []int{-1, 0, 2} {
		c := NewLinkContext(&obj.Link{Pkgpath: "p"})
		c.Experiment.LoopVar = true
		d := DefaultDebugFlags()
		d.LoopVar = value
		if err := c.PrepareNativeFlags(&d); err != nil {
			t.Fatal(err)
		}
		want := value
		if want == 0 {
			want = 1
		}
		if d.LoopVar != want {
			t.Fatalf("loopvar %d became %d", value, d.LoopVar)
		}
	}
	c := NewLinkContext(&obj.Link{Pkgpath: "runtime"})
	c.Flag.Std = true
	c.Flag.N = 1
	c.Flag.Race = true
	c.Flag.LowerC = 4
	d := DefaultDebugFlags()
	d.Checkptr = 2
	if err := c.PrepareNativeFlags(&d); err != nil {
		t.Fatal(err)
	}
	if !c.Flag.CompilingRuntime || c.Flag.N != 0 || !c.Ctxt.Flag_optimize || d.Checkptr != 0 || c.Flag.LowerC != 1 {
		t.Fatal("native runtime policy", d, c.Flag)
	}
}

func TestNativeDebugPolicyErrors(t *testing.T) {
	for _, value := range []string{"invalid-pattern", "-/bad", "0//1"} {
		c := NewLinkContext(&obj.Link{})
		d := DefaultDebugFlags()
		d.Fmahash = value
		if err := c.PrepareNativeFlags(&d); err == nil {
			t.Fatalf("invalid hash %q accepted", value)
		}
	}
	c := NewLinkContext(&obj.Link{})
	d := DefaultDebugFlags()
	d.FIPSHash = "?"
	if err := c.PrepareNativeFlags(&d); err == nil {
		t.Fatal("invalid FIPS matcher accepted")
	}
}

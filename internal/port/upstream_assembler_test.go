package port

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestPinnedAssembler offers a focused offline check of the complete native machine-code assembly pipeline.
// The complete upgrade suite also executes these snapshots.
func TestPinnedAssembler(t *testing.T) {
	repo := os.Getenv("GO_SSA_SOURCE_REPO")
	if repo == "" {
		t.Skip("set GO_SSA_SOURCE_REPO for pinned machine-code assembly differential")
	}
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	for _, tag := range []string{"go1.27.1", "go1.27.0"} {
		t.Run(tag, func(t *testing.T) {
			pin := Baseline()
			if tag != pin.Tag {
				pin.Tag = tag
				commit, err := git(context.Background(), repo, nil, "rev-parse", "refs/tags/"+tag+"^{commit}")
				if err != nil {
					t.Fatal(err)
				}
				pin.Commit = strings.TrimSpace(string(commit))
			}
			s, err := OpenSource(context.Background(), repo, pin)
			if err != nil {
				t.Fatal(err)
			}
			r, err := Render(s, Migration(), root)
			if err != nil {
				t.Fatal(err)
			}
			testUpstreamNativeGraph(t, s, r, root, "ssagen", "ssagen_assemble")
		})
	}
}

// This temporary observer uses the exact native Flushplist body up to assembly.
// PCLN and DWARF emission are later stages and are deliberately excluded here.
func nativeAssemblyStageObserver(t *testing.T, s *Source) string {
	t.Helper()
	data, err := s.Read("src/cmd/internal/obj/plist.go")
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	start := strings.Index(text, "func Flushplist(")
	if start < 0 {
		t.Fatal("native Flushplist boundary changed")
	}
	end := strings.Index(text[start:], "\nfunc ")
	if end < 0 {
		t.Fatal("native Flushplist boundary changed")
	}
	body := text[start : start+end]
	for _, line := range []string{"\t\tlinkpcln(ctxt, s)\n", "\t\tctxt.populateDWARF(plist.Curfn, s)\n"} {
		if strings.Count(body, line) != 1 {
			t.Fatal("native assembly stage boundary changed")
		}
		body = strings.Replace(body, line, "", 1)
	}
	body = strings.Replace(body, "func Flushplist(", "func SyncgoAssembleStage(", 1)
	return `package obj;import("fmt";"strings";"cmd/internal/objabi";"internal/abi");` + body
}
func assemblySnapshotObserver(code string, enabled bool) string {
	run, symbols := "", ""
	if enabled {
		run = `pp.Text.Ctxt.DiagFunc=func(f string,args ...any){panic(fmt.Sprintf(f,args...))};obj.SyncgoAssembleStage(pp.Text.Ctxt,&obj.Plist{Firstpc:pp.Text,Curfn:fn},pp.NewProg);if pp.Text.Ctxt.Errors!=0{panic("assembly failed")}`
		symbols = `appendSym("machine",fn.LSym);for _,jt:=range fi.JumpTables{appendSym("jumpbytes",jt.Sym)}`
	}
	return strings.NewReplacer("ASSEMBLE_RUN", run, "ASSEMBLE_SYMBOL", symbols).Replace(code)
}

package port

import (
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestPinnedCompileQueue offers a focused offline check of the native function queue and package archives.
// The complete upgrade suite also executes these snapshots.
func TestPinnedCompileQueue(t *testing.T) {
	repo := os.Getenv("GO_SSA_SOURCE_REPO")
	if repo == "" {
		t.Skip("set GO_SSA_SOURCE_REPO for pinned compile queue differential")
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
			testUpstreamNativeGraph(t, s, r, root, "ssagen", "ssagen_queue")
		})
	}
}

func queueSnapshotFixture(t *testing.T, code string, native bool) string {
	t.Helper()
	code = replaceFlushFixture(t, code, `raw:="ref example/p.Grow ABI0\n"`, `raw:="ref example/p.Grow ABI0\ndef example/p.Bodyless ABI0\n"`)
	if native {
		code = replaceFlushFixture(t, code, `base.Flag.LowerC=1;`, `base.Flag.LowerC=1;if strings.Contains(mode,"workers4"){base.Flag.LowerC=4};`)
	} else {
		code = replaceFlushFixture(t, code, `graph.Base.Flag.LowerC=1;`, `graph.Base.Flag.LowerC=1;if strings.Contains(mode,"workers4"){graph.Base.Flag.LowerC=4};`)
	}
	start := strings.Index(code, " var walked []*ir.Func")
	end := strings.Index(code, " for _,sym:=range ")
	if start < 0 || end < start {
		t.Fatal("queue fixture boundaries changed")
	}
	run := `backend.Inline=c.Inline;finalizer:=gc.NewContext(backend.SSA.ReflectData.Runtime.Writer);finalizer.BindPackage(packageInit);finalizer.BindBackend(backend,w);for _,fn:=range tc.Target.Funcs{finalizer.EnqueueFunc(fn,s)};finalizer.CompileQueued(nil);backend.CheckLargeStacks();if finalizer.QueuedFunctions()!=0||c.Diagnostics.HasErrors(){t.Fatal("native queue did not complete")};`
	if native {
		run = `for _,fn:=range typecheck.Target.Funcs{enqueueFunc(fn,s)};compileFunctions(nil);ssagen.CheckLargeStacks();if len(compilequeue)!=0||base.Errors()!=0{t.Fatal("native queue did not complete")};`
	}
	code = code[:start] + run + code[end:]
	if native {
		code = replaceFlushFixture(t, code, `"cmd/compile/internal/amd64";`, `"cmd/compile/internal/amd64";"cmp";"internal/race";"math/rand";"slices";"sync";"cmd/compile/internal/liveness";"cmd/compile/internal/pgoir";"cmd/compile/internal/staticinit";`)
	} else {
		// The same finalizer owns both the queue and the archive stage.
		code = replaceFlushFixture(t, code, `finalizer:=gc.NewContext(backend.SSA.ReflectData.Runtime.Writer);finalizer.BindPackage(packageInit);for _,n:=range tc.Target.Externs`, `for _,n:=range tc.Target.Externs`)
	}
	return code
}

// Copy all native queue declarations; no queue body or scheduling algorithm is edited.
func nativeGCQueueObserver(t *testing.T, s *Source) string {
	t.Helper()
	data, err := s.Read("src/cmd/compile/internal/gc/compile.go")
	if err != nil {
		t.Fatal(err)
	}
	fs := token.NewFileSet()
	f, err := parser.ParseFile(fs, "compile.go", data, 0)
	if err != nil {
		t.Fatal(err)
	}
	var out strings.Builder
	for _, d := range f.Decls {
		if g, ok := d.(*ast.GenDecl); ok && g.Tok == token.IMPORT {
			continue
		}
		out.WriteString("\n" + string(data[fs.Position(d.Pos()).Offset:fs.Position(d.End()).Offset]) + "\n")
	}
	return out.String()
}

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

// TestPinnedPackageFinish offers a focused offline check of the native function queue and package archives.
// The complete upgrade suite also executes these snapshots.
func TestPinnedPackageFinish(t *testing.T) {
	repo := os.Getenv("GO_SSA_SOURCE_REPO")
	if repo == "" {
		t.Skip("set GO_SSA_SOURCE_REPO for pinned package finalization differential")
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
			testUpstreamNativeGraph(t, s, r, root, "ssagen", "ssagen_package")
		})
	}
}

func packageFinishSnapshotFixture(t *testing.T, code string, native bool) string {
	t.Helper()
	if native {
		code = replaceFlushFixture(t, code, `noder.SyncgoReadInit(string(data))`, `base.Debug.WrapGlobalMapCtl=2;if strings.Contains(mode,"nooutline"){base.Debug.WrapGlobalMapCtl=1};noder.SyncgoReadInit(string(data))`)
		code = replaceFlushFixture(t, code, `dumpdata();`, `SyncgoFinishPackage();`)
		code = replaceFlushFixture(t, code, `base.Ctxt.NumberSyms();`, ``)
	} else {
		code = replaceFlushFixture(t, code, `c.ReadLocalPackage(string(data))`, `c.Debug.WrapGlobalMapCtl=2;if strings.Contains(mode,"nooutline"){c.Debug.WrapGlobalMapCtl=1};c.ReadLocalPackage(string(data))`)
		code = replaceFlushFixture(t, code, `finalizer.DumpData();`, `finalizer.FinishPackage();if !link.ObjectNumbered(){t.Fatal("finish did not number package")};`)
	}
	return code
}

// Observe the original post-compilation interval independently of migration.
func nativeGCFinishObserver(t *testing.T, s *Source) string {
	t.Helper()
	data, e := s.Read("src/cmd/compile/internal/gc/main.go")
	if e != nil {
		t.Fatal(e)
	}
	fs := token.NewFileSet()
	f, e := parser.ParseFile(fs, "main.go", data, 0)
	if e != nil {
		t.Fatal(e)
	}
	first, last := -1, -1
	for _, d := range f.Decls {
		fn, ok := d.(*ast.FuncDecl)
		if !ok || fn.Name.Name != "Main" {
			continue
		}
		for _, stmt := range fn.Body.List {
			start, end := fs.Position(stmt.Pos()).Offset, fs.Position(stmt.End()).Offset
			text := string(data[start:end])
			if strings.HasPrefix(text, "if base.Flag.CompilingRuntime") && strings.Contains(text, "ssagen.NoWriteBarrierRecCheck()") {
				if first >= 0 {
					t.Fatal("ambiguous finish start")
				}
				first = start
			}
			if text == "base.Ctxt.NumberSyms()" {
				if last >= 0 {
					t.Fatal("ambiguous finish end")
				}
				last = end
			}
		}
	}
	if first < 0 || last <= first {
		t.Fatal("native finish boundaries changed")
	}
	return "\nfunc SyncgoFinishPackage(){\n" + string(data[first:last]) + "\n}\n"
}

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

// TestPinnedFrontendPipeline offers a focused offline check of the native function queue and package archives.
// The complete upgrade suite also executes these snapshots.
func TestPinnedFrontendPipeline(t *testing.T) {
	repo := os.Getenv("GO_SSA_SOURCE_REPO")
	if repo == "" {
		t.Skip("set GO_SSA_SOURCE_REPO for pinned frontend pipeline differential")
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
			testUpstreamNativeGraph(t, s, r, root, "ssagen", "ssagen_pipeline")
		})
	}
}

func frontendPipelineSnapshotFixture(t *testing.T, code string, native bool) string {
	t.Helper()
	if native {
		code = replaceFlushFixture(t, code, `interleaved.DevirtualizeAndInlinePackage(typecheck.Target,nil);noder.MakeWrappers(typecheck.Target);for _,fn:=range typecheck.Target.Funcs{loopvar.ForCapture(fn)}`, ``)
		code = replaceFlushFixture(t, code, `pkginit.MakeTask()`, ``)
		code = replaceFlushFixture(t, code, `deadlocals.Funcs(typecheck.Target.Funcs)`, ``)
		code = replaceFlushFixture(t, code, `escape.Funcs(typecheck.Target.Funcs)`, ``)
		code = replaceFlushFixture(t, code, `slice.Funcs(typecheck.Target.Funcs)`, ``)
		code = replaceFlushFixture(t, code, `SyncgoCompileDeclarations(s,nil);`, `SyncgoPrepareFrontend(s,nil);SyncgoCompileDeclarations(s,nil);`)
		code = replaceFlushFixture(t, code, `"cmd/compile/internal/amd64";`, `"cmd/compile/internal/amd64";"cmd/compile/internal/bloop";`)
	} else {
		code = replaceFlushFixture(t, code, `c.Interleaved.DevirtualizeAndInlinePackage(tc.Target,nil);c.MakeWrappers(tc.Target);capture:=loopvar.NewContext(tc,c.Inline.Log);for _,fn:=range tc.Target.Funcs{capture.ForCapture(fn)}`, ``)
		code = replaceFlushFixture(t, code, `packageInit.MakeTask()`, ``)
		code = replaceFlushFixture(t, code, `deadlocals.NewContext(graph).Funcs(tc.Target.Funcs)`, ``)
		code = replaceFlushFixture(t, code, `esc.Funcs(tc.Target.Funcs)`, ``)
		code = replaceFlushFixture(t, code, `slice.NewContext(esc,c.ReflectData).Funcs(tc.Target.Funcs)`, ``)
		code = replaceFlushFixture(t, code, `finalizer.CompileDeclarations(s,nil);`, `finalizer.PrepareFrontend(s,nil);finalizer.CompileDeclarations(s,nil);`)
		for _, pkg := range []string{"loopvar", "deadlocals", "slice"} {
			code = replaceFlushFixture(t, code, `"go-backend-ssa/internal/`+pkg+`";`, ``)
		}
	}
	code = replaceFlushFixture(t, code, `s.GenABIWrappers()`, ``)
	// This assertion belongs after PrepareFrontend; code emission consumes inline marks.
	start := strings.Index(code, ` if strings.Contains(mode,"-inline"){seen:=false;`)
	if start < 0 {
		t.Fatal("inline assertion missing")
	}
	end := strings.Index(code[start:], "\n") + start
	assertion := code[start:end]
	code = code[:start] + code[end:]
	marker := `finalizer.PrepareFrontend(s,nil);`
	if native {
		marker = `SyncgoPrepareFrontend(s,nil);`
	}
	code = replaceFlushFixture(t, code, marker, marker+assertion+";\n")
	return code
}

// Read the untouched Main interval independently of the migration extractor.
func nativeGCFrontendObserver(t *testing.T, s *Source) string {
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
			if strings.HasPrefix(text, "for _, fn := range typecheck.Target.Funcs") && strings.Contains(text, `ir.MatchAstDump(fn, "start")`) {
				if first >= 0 {
					t.Fatal("ambiguous frontend start")
				}
				first = start
			}
			if text == "reflectdata.WriteBasicTypes()" {
				if last >= 0 {
					t.Fatal("ambiguous frontend end")
				}
				last = start
			}
		}
	}
	if first < 0 || last <= first {
		t.Fatal("native frontend boundaries changed")
	}
	return "\nfunc SyncgoPrepareFrontend(symABIs *ssagen.SymABIs, profile *pgoir.Profile){\n" + string(data[first:last]) + "\n}\n"
}

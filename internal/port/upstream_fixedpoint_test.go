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

// TestPinnedDeclarationFixedPoint offers a focused offline check of the native function queue and package archives.
// The complete upgrade suite also executes these snapshots.
func TestPinnedDeclarationFixedPoint(t *testing.T) {
	repo := os.Getenv("GO_SSA_SOURCE_REPO")
	if repo == "" {
		t.Skip("set GO_SSA_SOURCE_REPO for pinned declaration fixed point differential")
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
			testUpstreamNativeGraph(t, s, r, root, "ssagen", "ssagen_fixed")
		})
	}
}

func fixedPointSnapshotFixture(t *testing.T, code string, native bool) string {
	t.Helper()
	if native {
		code = replaceFlushFixture(t, code, `for _,fn:=range typecheck.Target.Funcs{enqueueFunc(fn,s)};compileFunctions(nil);`, `SyncgoCompileDeclarations(s,nil);`)
		code = replaceFlushFixture(t, code, `for _,n:=range typecheck.Target.Externs{switch n.Op(){case ir.ONAME:SyncgoDumpGlobal(n);case ir.OLITERAL:SyncgoDumpGlobalConst(n)}};`, ``)
		code = replaceFlushFixture(t, code, `base.Ctxt.DwFixups.Finalize(base.Ctxt.Pkgpath,false);`, `if base.Ctxt.DwFixups!=nil||base.Flag.GenDwarfInl!=0{t.Fatal("native fixed point did not finalize DWARF")};`)
		code = replaceFlushFixture(t, code, `reflectdata.WriteGCSymbols();staticdata.WriteFuncSyms();SyncgoAddGCLocals();`, `dumpdata();`)
	} else {
		code = replaceFlushFixture(t, code, `for _,fn:=range tc.Target.Funcs{finalizer.EnqueueFunc(fn,s)};finalizer.CompileQueued(nil);`, `finalizer.CompileDeclarations(s,nil);`)
		code = replaceFlushFixture(t, code, `for _,n:=range tc.Target.Externs{switch n.Op(){case ir.ONAME:finalizer.DumpGlobal(n);case ir.OLITERAL:finalizer.DumpGlobalConst(n)}};`, ``)
		code = replaceFlushFixture(t, code, `link.DwFixups.Finalize(link.Pkgpath,false);`, `if link.DwFixups!=nil||c.Flag.GenDwarfInl!=0{t.Fatal("fixed point did not finalize DWARF")};`)
		code = replaceFlushFixture(t, code, `backend.SSA.ReflectData.WriteGCSymbols();backend.SSA.ReflectData.StaticData.WriteFuncSyms();finalizer.AddGCLocals();`, `finalizer.DumpData();`)
	}
	return code
}

// The native observer takes the entire original declaration interval, including
// the fixed point and its DWARF continuation, without using the port extractor.
func nativeGCFixedPointObserver(t *testing.T, s *Source) string {
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
	var body string
	for _, d := range f.Decls {
		fn, ok := d.(*ast.FuncDecl)
		if !ok || fn.Name.Name != "Main" {
			continue
		}
		first, last := -1, -1
		for _, stmt := range fn.Body.List {
			start, end := fs.Position(stmt.Pos()).Offset, fs.Position(stmt.End()).Offset
			text := string(data[start:end])
			if text == "reflectdata.WriteBasicTypes()" {
				if first >= 0 {
					t.Fatal("ambiguous native interval")
				}
				first = start
			}
			if text == `base.Timer.AddEvent(int64(len(typecheck.Target.Funcs)), "funcs")` {
				if last >= 0 {
					t.Fatal("ambiguous native interval")
				}
				last = end
			}
		}
		if first < 0 || last <= first {
			t.Fatal("native interval changed")
		}
		body = string(data[first:last])
	}
	if body == "" {
		t.Fatal("native Main missing")
	}
	return "\nfunc SyncgoCompileDeclarations(symABIs *ssagen.SymABIs, profile *pgoir.Profile){\n" + body + "\n}\nfunc dumpGlobal(n *ir.Name){SyncgoDumpGlobal(n)}\nfunc dumpGlobalConst(n *ir.Name){SyncgoDumpGlobalConst(n)}\n"
}

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

func TestPinnedPackageData(t *testing.T) {
	repo := os.Getenv("GO_SSA_SOURCE_REPO")
	if repo == "" {
		t.Skip("set GO_SSA_SOURCE_REPO for pinned package data differential")
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
				commit, e := git(context.Background(), repo, nil, "rev-parse", "refs/tags/"+tag+"^{commit}")
				if e != nil {
					t.Fatal(e)
				}
				pin.Commit = strings.TrimSpace(string(commit))
			}
			s, e := OpenSource(context.Background(), repo, pin)
			if e != nil {
				t.Fatal(e)
			}
			r, e := Render(s, Migration(), root)
			if e != nil {
				t.Fatal(e)
			}
			testUpstreamNativeGraph(t, s, r, root, "staticdata")
			testUpstreamNativeGraph(t, s, r, root, "ssagen", "ssagen_data")
		})
	}
}

func dataSnapshotFixture(t *testing.T, code string, native bool) string {
	t.Helper()
	setup := `inputData:=map[string]string{"small":"embedded bytes","large":strings.Repeat("streamed",512)};`
	if native {
		code = replaceFlushFixture(t, code, `reflectdata.WriteGCSymbols();staticdata.WriteFuncSyms();SyncgoAddGCLocals();`, `dumpdata();`)
		setup += `inputRoot:=t.TempDir();for name,data:=range inputData{if err:=os.WriteFile(filepath.Join(inputRoot,name),[]byte(data),0600);err!=nil{t.Fatal(err)}};`
	} else {
		code = replaceFlushFixture(t, code, `backend.SSA.ReflectData.WriteGCSymbols();backend.SSA.ReflectData.StaticData.WriteFuncSyms();finalizer.AddGCLocals();`, `finalizer.DumpData();`)
		code = replaceFlushFixture(t, code, `"go-backend-ssa/internal/amd64";`, `"go-backend-ssa/internal/amd64";"go-backend-ssa/internal/cmd/src";"io";"io/fs";"testing/fstest";`)
		setup += `inputFS:=fstest.MapFS{};for name,data:=range inputData{inputFS[name]=&fstest.MapFile{Data:[]byte(data)}};c.ReflectData.StaticData.OpenFile=inputFS.Open;link.OpenObjectInput=func(p string)(io.ReadCloser,error){return inputFS.Open(p)};_ = fs.ValidPath;`
	}
	setup += `FLAG.Cfg.Embed.Patterns=map[string][]string{"small":{"small"},"large":{"large"}};FLAG.Cfg.Embed.Files=map[string]string{"small":PATH_SMALL,"large":PATH_LARGE};for i,typ:=range []*types.Type{TY.Types[types.TSTRING],TY.NewSlice(TY.Types[types.TUINT8]),TY.Types[types.TSTRING],TY.NewSlice(TY.Types[types.TUINT8])}{pattern:="small";if i>=2{pattern="large"};n:=GRAPH.NewNameAt(src.XPos{},TY.LocalPkg.Lookup(fmt.Sprint("Embedded",i)),typ);n.Class=ir.PEXTERN;n.Embed=&[]ir.Embed{{Patterns:[]string{pattern}}};TARGET.Embeds=append(TARGET.Embeds,n);TARGET.Externs=append(TARGET.Externs,n);if i==0{TARGET.PluginExports=append(TARGET.PluginExports,n)}};REFLECT.ZeroSize=257;`
	before := `for _,n:=range tc.Target.Externs`
	if native {
		setup = strings.NewReplacer("FLAG", "base.Flag", "TY.", "types.", "GRAPH.", "ir.", "TARGET", "typecheck.Target", "REFLECT", "reflectdata", "PATH_SMALL", `filepath.Join(inputRoot,"small")`, "PATH_LARGE", `filepath.Join(inputRoot,"large")`).Replace(setup)
		before = `for _,n:=range typecheck.Target.Externs`
	} else {
		setup = strings.NewReplacer("FLAG", "c.Flag", "TY.", "u.", "GRAPH.", "graph.", "TARGET", "tc.Target", "REFLECT", "c.ReflectData", "PATH_SMALL", `"small"`, "PATH_LARGE", `"large"`).Replace(setup)
	}
	return replaceFlushFixture(t, code, before, setup+before)
}

// Execute the complete pinned data-stage bodies in the native observer package.
func nativeGCDataObserver(t *testing.T, s *Source) string {
	t.Helper()
	data, e := s.Read("src/cmd/compile/internal/gc/obj.go")
	if e != nil {
		t.Fatal(e)
	}
	fs := token.NewFileSet()
	f, e := parser.ParseFile(fs, "obj.go", data, 0)
	if e != nil {
		t.Fatal(e)
	}
	out := "\nfunc addGCLocals(){SyncgoAddGCLocals()}\n"
	found := 0
	for _, d := range f.Decls {
		if fn, ok := d.(*ast.FuncDecl); ok && (fn.Name.Name == "dumpdata" || fn.Name.Name == "dumpembeds") {
			out += string(data[fs.Position(fn.Pos()).Offset:fs.Position(fn.End()).Offset]) + "\n"
			found++
		}
	}
	if found != 2 {
		t.Fatal("native data bodies missing")
	}
	return out
}

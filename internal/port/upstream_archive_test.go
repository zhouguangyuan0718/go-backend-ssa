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

// TestPinnedArchive offers a focused offline check of the complete native object file pipeline.
// The complete upgrade suite also executes these snapshots.
func TestPinnedArchive(t *testing.T) {
	repo := os.Getenv("GO_SSA_SOURCE_REPO")
	if repo == "" {
		t.Skip("set GO_SSA_SOURCE_REPO for pinned package archive differential")
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
			testUpstreamNativeGraph(t, s, r, root, "ssagen", "ssagen_archive")
		})
	}
}

func archiveSnapshotFixture(t *testing.T, code string, native bool, version string) string {
	t.Helper()
	if native {
		code = replaceFlushFixture(t, code, `"cmd/internal/bio";`, `"cmd/internal/bio";"cmd/internal/archive";"internal/goexperiment";`)
		code = replaceFlushFixture(t, code, `mode:=os.Getenv("SYNCGO_TARGET")`, `mode:=os.Getenv("SYNCGO_TARGET");buildcfg.Experiment.Flags=goexperiment.Flags{}`)
		old := `objectPath:=filepath.Join(t.TempDir(),"native.o");objectOut,err:=bio.Create(objectPath);if err!=nil{t.Fatal(err)};base.Ctxt.NumberSyms();obj.WriteObjFile(base.Ctxt,objectOut);if err:=objectOut.Close();err!=nil{t.Fatal(err)};objectBytes,err:=os.ReadFile(objectPath);if err!=nil{t.Fatal(err)};`
		replacement := `if strings.Contains(mode,"archive-cgo"){typecheck.Target.CgoPragmas=[][]string{{"cgo_import_dynamic","probe","probe","libprobe.so"}}};if strings.Contains(mode,"archive-buildid"){base.Flag.BuildID="test/archive build id"};objectPath:=filepath.Join(t.TempDir(),"native.a");base.Ctxt.NumberSyms();if strings.Contains(mode,"archive-split"){compilerPath:=filepath.Join(t.TempDir(),"compiler.a");SyncgoDumpArchive(compilerPath,modeCompilerObj);compilerBytes,err:=os.ReadFile(compilerPath);if err!=nil{t.Fatal(err)};for i:=0;i<len(compilerBytes);i+=256{out=append(out,fmt.Sprintf("compiler:%08x:%x",i,compilerBytes[i:min(i+256,len(compilerBytes))]))};SyncgoDumpArchive(objectPath,modeLinkerObj)}else{SyncgoDumpArchive(objectPath,modeCompilerObj|modeLinkerObj)};objectBytes,err:=os.ReadFile(objectPath);if err!=nil{t.Fatal(err)};`
		code = replaceFlushFixture(t, code, old, replacement)
		readback := `archiveInput,err:=os.Open(objectPath);if err!=nil{t.Fatal(err)};parsedArchive,err:=archive.Parse(archiveInput,false);if err!=nil{t.Fatal(err)};if err:=archiveInput.Close();err!=nil{t.Fatal(err)};wantMembers:=2;if strings.Contains(mode,"archive-split"){wantMembers=1};if len(parsedArchive.Entries)!=wantMembers{t.Fatal("archive member count",len(parsedArchive.Entries))};lastMember:=parsedArchive.Entries[len(parsedArchive.Entries)-1];if lastMember.Name!="_go_.o"||lastMember.Obj==nil{t.Fatal("linker object member missing")};func(objectBytes []byte){` + nativeObjectReadback() + `}(objectBytes[lastMember.Obj.Offset:lastMember.Obj.Offset+lastMember.Obj.Size]);`
		code = replaceFlushFixture(t, code, nativeObjectReadback(), readback)
	} else {
		old := `var objectOut syncgoObjectOutput;if err:=backend.SSA.ReflectData.Runtime.Writer.WriteObject(&objectOut);err!=nil{t.Fatal(err)};objectBytes:=objectOut.data;`
		replacement := `if strings.Contains(mode,"archive-cgo"){tc.Target.CgoPragmas=[][]string{{"cgo_import_dynamic","probe","probe","libprobe.so"}}};if strings.Contains(mode,"archive-buildid"){c.Flag.BuildID="test/archive build id"};finalizer.SetObjectHeader(gc.ObjectConfig{GOOS:"linux",GOARCH:"amd64",GOAMD64:1,Version:"` + version + `"});var objectOut syncgoObjectOutput;if strings.Contains(mode,"archive-split"){var compilerOut syncgoObjectOutput;if err:=finalizer.WriteArchive(&compilerOut,gc.CompilerObject);err!=nil{t.Fatal(err)};compilerBytes:=compilerOut.data;for i:=0;i<len(compilerBytes);i+=256{out=append(out,fmt.Sprintf("compiler:%08x:%x",i,compilerBytes[i:min(i+256,len(compilerBytes))]))};if err:=finalizer.WriteArchive(&objectOut,gc.LinkerObject);err!=nil{t.Fatal(err)}}else{if err:=finalizer.WriteArchive(&objectOut,gc.CombinedObject);err!=nil{t.Fatal(err)}};objectBytes:=objectOut.data;`
		code = replaceFlushFixture(t, code, old, replacement)
	}
	return code
}

// Use the complete pinned container functions on the native reference side.
// Only the dumpobj1 entry name changes to avoid inaccessible gc internals.
func nativeGCArchiveObserver(t *testing.T, s *Source) string {
	t.Helper()
	data, err := s.Read("src/cmd/compile/internal/gc/obj.go")
	if err != nil {
		t.Fatal(err)
	}
	fs := token.NewFileSet()
	file, err := parser.ParseFile(fs, "obj.go", data, 0)
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{"dumpobj1": true, "printObjHeader": true, "startArchiveEntry": true, "finishArchiveEntry": true, "dumpCompilerObj": true, "dumpLinkerObj": true}
	var out strings.Builder
	found := 0
	for _, decl := range file.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok && names[fn.Name.Name] {
			body := string(data[fs.Position(fn.Pos()).Offset:fs.Position(fn.End()).Offset])
			if fn.Name.Name == "dumpobj1" {
				body = strings.Replace(body, "func dumpobj1(", "func SyncgoDumpArchive(", 1)
			}
			out.WriteString("\n" + body + "\n")
			found++
		}
		if gen, ok := decl.(*ast.GenDecl); ok && gen.Tok == token.CONST {
			for _, spec := range gen.Specs {
				v := spec.(*ast.ValueSpec)
				if len(v.Names) > 0 && v.Names[0].Name == "modeCompilerObj" {
					out.WriteString("\n" + string(data[fs.Position(gen.Pos()).Offset:fs.Position(gen.End()).Offset]) + "\n")
					found++
				}
			}
		}
	}
	if found != len(names)+1 {
		t.Fatal("native archive declarations missing")
	}
	return out.String()
}

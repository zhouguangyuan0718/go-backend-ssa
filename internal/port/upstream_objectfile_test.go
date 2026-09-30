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

// TestPinnedObjectFile offers a focused offline check of the complete native object file pipeline.
// The complete upgrade suite also executes these snapshots.
func TestPinnedObjectFile(t *testing.T) {
	repo := os.Getenv("GO_SSA_SOURCE_REPO")
	if repo == "" {
		t.Skip("set GO_SSA_SOURCE_REPO for pinned object file differential")
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
			testUpstreamNativeGraph(t, s, r, root, "ssagen", "ssagen_object")
		})
	}
}

func objectSnapshotFixture(t *testing.T, code string, native bool) string {
	t.Helper()
	var encode string
	if native {
		code = replaceFlushFixture(t, code, `"cmd/compile/internal/amd64";`, `"cmd/compile/internal/amd64";"cmd/internal/bio";"cmd/internal/goobj";"cmd/compile/internal/reflectdata";"cmd/compile/internal/staticdata";"cmd/compile/internal/objw";"cmd/internal/objabi";"path/filepath";`)
		encode = `for _,n:=range typecheck.Target.Externs{switch n.Op(){case ir.ONAME:SyncgoDumpGlobal(n);case ir.OLITERAL:SyncgoDumpGlobalConst(n)}};reflectdata.WriteGCSymbols();staticdata.WriteFuncSyms();SyncgoAddGCLocals();objectPath:=filepath.Join(t.TempDir(),"native.o");objectOut,err:=bio.Create(objectPath);if err!=nil{t.Fatal(err)};base.Ctxt.NumberSyms();obj.WriteObjFile(base.Ctxt,objectOut);if err:=objectOut.Close();err!=nil{t.Fatal(err)};objectBytes,err:=os.ReadFile(objectPath);if err!=nil{t.Fatal(err)};` + nativeObjectReadback()
	} else {
		code = replaceFlushFixture(t, code, `"go-backend-ssa/internal/amd64";`, `"go-backend-ssa/internal/amd64";"go-backend-ssa/internal/gc";`)
		code = replaceFlushFixture(t, code, `pkginit.NewContext(c).MakeTask()`, `packageInit:=pkginit.NewContext(c);packageInit.MakeTask()`)
		encode = `finalizer:=gc.NewContext(backend.SSA.ReflectData.Runtime.Writer);finalizer.BindPackage(packageInit);for _,n:=range tc.Target.Externs{switch n.Op(){case ir.ONAME:finalizer.DumpGlobal(n);case ir.OLITERAL:finalizer.DumpGlobalConst(n)}};backend.SSA.ReflectData.WriteGCSymbols();backend.SSA.ReflectData.StaticData.WriteFuncSyms();finalizer.AddGCLocals();var objectOut syncgoObjectOutput;if err:=backend.SSA.ReflectData.Runtime.Writer.WriteObject(&objectOut);err!=nil{t.Fatal(err)};objectBytes:=objectOut.data;`
		code += objectMemoryFixture()
	}
	encode += `for start:=0;start<len(objectBytes);start+=256{end:=min(start+256,len(objectBytes));out=append(out,fmt.Sprintf("object:%08x:%x",start,objectBytes[start:end]))};`
	return replaceFlushFixture(t, code, "b,err:=json.Marshal(out)", encode+"b,err:=json.Marshal(out)")
}
func objectMemoryFixture() string {
	return `
type syncgoObjectOutput struct{data []byte;offset int64}
func(b *syncgoObjectOutput)Write(p []byte)(int,error){end:=b.offset+int64(len(p));if end>int64(len(b.data)){b.data=append(b.data,make([]byte,end-int64(len(b.data)))...)};copy(b.data[b.offset:end],p);b.offset=end;return len(p),nil}
func(b *syncgoObjectOutput)Seek(n int64,whence int)(int64,error){switch whence{case 0:case 1:n+=b.offset;case 2:n+=int64(len(b.data));default:return 0,fmt.Errorf("whence %d",whence)};if n<0{return 0,fmt.Errorf("negative offset")};b.offset=n;return n,nil}
`
}

// Decode every object table with the pinned official reader on the reference
// side. Exact payload equality then applies this validation to migrated output.
func nativeObjectReadback() string {
	return `
objectReader:=goobj.NewReaderFromBytes(objectBytes,false);if objectReader==nil||objectReader.NSym()==0{t.Fatal("native object reader rejected payload")}
objectDefs:=objectReader.NSym()+objectReader.NHashed64def()+objectReader.NHasheddef()+objectReader.NNonpkgdef()
checkRef:=func(ref goobj.SymRef){limit:=-1;switch ref.PkgIdx{case 0:if ref.SymIdx!=0{t.Fatal("invalid nil symbol")};return;case goobj.PkgIdxSelf:limit=objectReader.NSym();case goobj.PkgIdxHashed64:limit=objectReader.NHashed64def();case goobj.PkgIdxHashed:limit=objectReader.NHasheddef();case goobj.PkgIdxNone:limit=objectReader.NNonpkgdef()+objectReader.NNonpkgref();case goobj.PkgIdxBuiltin:limit=goobj.NBuiltin();default:if int(ref.PkgIdx)>=objectReader.NPkg(){t.Fatal("invalid package reference",ref)};return};if int(ref.SymIdx)>=limit{t.Fatal("invalid symbol reference",ref,limit)}}
for i:=0;i<objectReader.NFile();i++{_ =objectReader.File(i)}
for i:=0;i<objectDefs+objectReader.NNonpkgref();i++{_ =objectReader.Sym(uint32(i)).Name(objectReader)}
for i:=0;i<objectDefs;i++{idx:=uint32(i);_ =objectReader.Data(idx);for _,r:=range objectReader.Relocs(idx){checkRef(r.Sym())};for _,a:=range objectReader.Auxs(idx){checkRef(a.Sym())}}
for i:=0;i<objectReader.NRefName();i++{ref:=objectReader.RefName(i);_ =ref.Name(objectReader);checkRef(ref.Sym())}
for i:=0;i<objectReader.NRefFlags();i++{checkRef(objectReader.RefFlags(i).Sym())}
`
}

// Run the complete pinned gc helper in the external reference test package. The
// function body and its base/obj/objw bindings are copied without edits.
func nativeGCObjectObserver(t *testing.T, s *Source) string {
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
	var out strings.Builder
	found := 0
	names := map[string]string{"addGCLocals": "SyncgoAddGCLocals", "dumpGlobal": "SyncgoDumpGlobal", "dumpGlobalConst": "SyncgoDumpGlobalConst", "ggloblnod": "ggloblnod"}
	for _, decl := range file.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok {
			if name, selected := names[fn.Name.Name]; selected {
				body := string(data[fs.Position(fn.Pos()).Offset:fs.Position(fn.End()).Offset])
				out.WriteString("\n" + strings.Replace(body, "func "+fn.Name.Name+"(", "func "+name+"(", 1) + "\n")
				found++
			}
		}
	}
	if found != len(names) {
		t.Fatal("native gc finalization helper missing")
	}
	return out.String()
}

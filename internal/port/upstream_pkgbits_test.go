package port

import (
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"testing"
)

// Compare the complete native codec with the migrated codec, including reads
// in both directions. The reference source only changes its test import path.
func testUpstreamPkgBits(t *testing.T, s *Source, r *Rendered, root string) {
	t.Helper()
	dir := t.TempDir()
	for name, data := range r.Files {
		if strings.HasPrefix(name, "internal/go/pkgbits/") {
			writeTestFile(t, dir, name, string(data))
		}
	}
	adapter, err := os.ReadFile(filepath.Join(root, "internal/go/pkgbits/adapter_decoder.go"))
	if err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, dir, "internal/go/pkgbits/adapter_decoder.go", string(adapter))
	for _, name := range sortedKeys(s.Blobs) {
		if path.Dir(name) != "src/internal/pkgbits" || !strings.HasSuffix(name, ".go") {
			continue
		}
		data, err := s.Read(name)
		if err != nil {
			t.Fatal(err)
		}
		dest := "internal/reference/pkgbits/" + path.Base(name)
		text := strings.ReplaceAll(string(data), `"internal/pkgbits"`, `"go-backend-ssa/internal/reference/pkgbits"`)
		writeTestFile(t, dir, dest, text)
		if strings.HasSuffix(name, "_test.go") {
			writeTestFile(t, dir, "internal/go/pkgbits/upstream_test.go", strings.ReplaceAll(string(data), `"internal/pkgbits"`, `"go-backend-ssa/internal/go/pkgbits"`))
		}
	}
	writeTestFile(t, dir, "go.mod", "module go-backend-ssa\n\ngo 1.27.1\n")
	header := `package compare
import("fmt";"go/constant";"go/token";"math/big";"reflect";"strings";"testing";ported "go-backend-ssa/internal/go/pkgbits";original "go-backend-ssa/internal/reference/pkgbits")
`
	template := `
func NAMEWrite(version int,frames int)string{
 pw:=API.NewPkgEncoder(API.Version(version),frames)
 p:=pw.NewEncoder(API.SectionPkg,API.SyncPkgDef);p.String("");p.Flush()
 o:=pw.NewEncoder(API.SectionName,API.SyncObject1);o.Sync(API.SyncSym);o.Sync(API.SyncPkg);o.Reloc(API.SectionPkg,0);o.String("Answer");o.Code(API.ObjConst);o.Flush()
 for range 3 {
  w:=pw.NewEncoder(API.SectionMeta,API.SyncPublic)
  w.Bool(true);w.Bool(false);w.Int64(-1<<63);w.Uint64(1<<64-1)
  w.Strings([]string{"世界","same","same","\x00"})
  values:=[]constant.Value{constant.MakeBool(true),constant.MakeString("text"),constant.MakeInt64(-42),constant.Make(new(big.Int).Lsh(big.NewInt(1),200)),constant.Make(new(big.Rat).SetFrac64(-7,3)),constant.Make(new(big.Float).SetPrec(512).SetFloat64(1.25)),constant.BinaryOp(constant.MakeInt64(2),token.ADD,constant.MakeImag(constant.MakeInt64(3)))}
  w.Len(len(values));for _,v:=range values{w.Value(v)}
  w.Reloc(API.SectionPkg,0);w.Sync(API.SyncEOF);w.Flush()
 }
 var out strings.Builder;pw.DumpTo(&out);return out.String()
}
func NAMERead(data string)[]string{
 pr:=API.NewPkgDecoder("example/p",data)
 out:=[]string{fmt.Sprint(pr.Fingerprint(),pr.SyncMarkers(),pr.NumElems(API.SectionMeta),pr.TotalElems()),pr.PeekPkgPath(0)}
 p,n,c:=pr.PeekObj(0);out=append(out,fmt.Sprint(p,n,c))
 for i:=range 3 {
  r:=pr.TempDecoder(API.SectionMeta,API.RelElemIdx(i),API.SyncPublic)
  out=append(out,fmt.Sprint(r.Version(),r.Bool(),r.Bool(),r.Int64(),r.Uint64(),r.Strings()))
  for n:=r.Len();n>0;n--{v:=r.Value();out=append(out,fmt.Sprint(v.Kind(),":",v.ExactString()))}
  out=append(out,fmt.Sprint(r.Reloc(API.SectionPkg)));r.Sync(API.SyncEOF);pr.RetireDecoder(&r)
 }
 return out
}
`
	code := header + strings.NewReplacer("NAME", "ported", "API", "ported").Replace(template) + strings.NewReplacer("NAME", "original", "API", "original").Replace(template) + `
func failure(f func())(message string){defer func(){if p:=recover();p!=nil{message=fmt.Sprintf("%T:%v",p,p)}}();f();return}
func TestCodecDifferential(t *testing.T){
 for version:=0;version<=4;version++{for _,frames:=range []int{-1,0}{t.Run(fmt.Sprintf("v%d/frames%d",version,frames),func(t *testing.T){
  want,got:=originalWrite(version,frames),portedWrite(version,frames)
  if got!=want{t.Fatal("encoded bytes differ")}
  // V0 has no Flags field: native encoding accepts sync markers but cannot
  // advertise them to the decoder. Preserve this invalid-input failure too.
  if version==0&&frames==0{a,b:=failure(func(){originalRead(want)}),failure(func(){portedRead(got)});if a==""||a!=b{t.Fatalf("invalid V0 marker failure: original=%s ported=%s",a,b)};return}
  baseline:=originalRead(want)
  for _,data:=range []string{got,want}{if !reflect.DeepEqual(portedRead(data),baseline)||!reflect.DeepEqual(originalRead(data),baseline){t.Fatal("cross-codec decode differs")}}
 })}}
}
`
	writeTestFile(t, dir, "compare/codec_test.go", code)
	cmd := exec.Command("go", "test", "-race", "-v", "./...")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOTOOLCHAIN=go1.27.1", "GOWORK=off")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("pkgbits differential: %v\n%s", err, out)
	} else {
		t.Logf("%s pkgbits differential:\n%s", s.Pin.Tag, out)
	}
}

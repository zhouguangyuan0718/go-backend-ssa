package port

import (
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The reference uses the unmodified posMap/trimFilename/AbsFile algorithms.
// Host globals are configured explicitly in a test-only reference package;
// no reference algorithm is redirected through the library's Context rules.
func testUpstreamNoderPositions(t *testing.T, s *Source, r *Rendered, root string) {
	t.Helper()
	dir := t.TempDir()
	for name, data := range r.Files {
		writeTestFile(t, dir, name, string(data))
	}
	for _, name := range Migration().SupportFiles {
		data, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			t.Fatal(err)
		}
		writeTestFile(t, dir, name, string(data))
	}
	for _, u := range Migration().Units {
		if u.Context != nil && u.Context.Adapter != "" {
			data, err := os.ReadFile(filepath.Join(root, u.adapterPath()))
			if err != nil {
				t.Fatal(err)
			}
			writeTestFile(t, dir, u.adapterPath(), string(data))
		}
	}
	mapping := Migration().importMap(s.Pin.TargetModule)
	mapping["cmd/compile/internal/base"] = "go-backend-ssa/internal/reference/base"
	mapping["cmd/internal/objabi"] = "go-backend-ssa/internal/reference/objabi"
	mapping["internal/buildcfg"] = "go-backend-ssa/internal/reference/buildcfg"
	for _, u := range []Unit{
		{Source: "src/cmd/compile/internal/noder/posmap.go", WholeFile: true},
		{Source: "src/cmd/compile/internal/noder/noder.go", Functions: []string{"trimFilename"}},
		{Source: "src/cmd/internal/objabi/line.go", Functions: []string{"AbsFile", "ApplyRewrites", "applyRewrite", "hasPathPrefix"}},
	} {
		data, err := s.Read(u.Source)
		if err != nil {
			t.Fatal(err)
		}
		fs := token.NewFileSet()
		f, err := selectFile(fs, u.Source, data, u)
		if err != nil {
			t.Fatal(err)
		}
		pruneImports(f)
		code := nodeText(fs, f)
		for from, to := range mapping {
			code = strings.ReplaceAll(code, `"`+from+`"`, `"`+to+`"`)
		}
		code = strings.ReplaceAll(code, "src.NoXPos", "src.XPos{}")
		// Check the extracted reference remains valid Go independently of the port.
		if _, err := parser.ParseFile(token.NewFileSet(), u.Source, code, 0); err != nil {
			t.Fatal(err)
		}
		pkg := "noder"
		if strings.Contains(u.Source, "objabi") {
			pkg = "objabi"
		}
		writeTestFile(t, dir, "internal/reference/"+pkg+"/"+filepath.Base(u.Source), code)
	}
	writeTestFile(t, dir, "internal/reference/base/state.go", `package base
import("go-backend-ssa/internal/base";"go-backend-ssa/internal/cmd/obj")
var Flag base.CmdFlags
var Ctxt *obj.Link
`)
	writeTestFile(t, dir, "internal/reference/buildcfg/state.go", `package buildcfg
var GOROOT string
`)
	writeTestFile(t, dir, "go.mod", "module go-backend-ssa\n\ngo 1.27.1\n")
	writeTestFile(t, dir, "internal/reference/noder/compare_test.go", `package noder
import("fmt";"reflect";"testing";"go-backend-ssa/internal/reference/base";"go-backend-ssa/internal/reference/buildcfg";"go-backend-ssa/internal/cmd/obj";"go-backend-ssa/internal/cmd/src";"go-backend-ssa/internal/syntax";ported "go-backend-ssa/internal/noder")
func snapshot(link *obj.Link,mapPos func(syntax.Pos)src.XPos,trim func(*syntax.PosBase)string,filename string,trimmed bool)[]string{
 file:=syntax.NewTrimmedFileBase(filename,trimmed)
 line:=syntax.NewLineBase(syntax.MakePos(file,4,2),"virtual.go",false,40,3)
 nested:=syntax.NewLineBase(syntax.MakePos(line,6,4),"generated.go",true,70,5)
 out:=[]string{trim(file),trim(line),trim(nested)}
 for _,pos:=range []syntax.Pos{{},syntax.MakePos(file,1,1),syntax.MakePos(line,4,2),syntax.MakePos(nested,7,6),syntax.MakePos(file,100000,90000),syntax.MakePos(file,1,1)}{
  x:=mapPos(pos);p:=link.PosTable.Pos(x);out=append(out,fmt.Sprintf("%v:%s:%d:%d",x,p.String(),p.RelLine(),p.RelCol()))
  if p.IsKnown(){out=append(out,p.Base().AbsFilename())}
 }
 return out
}
func TestPositionDifferential(t *testing.T){
 for _,dir:=range []string{"","/work/src"}{for _,file:=range []string{"p.go","/sdk/go/src/runtime/p.go","/work/src/p.go","/workspace/p.go",""}{for _,rewrite:=range []string{"","/work=>example","/work=>first;/work/src=>second","/SDK/GO=>root","/work/src"}{for _,trimmed:=range []bool{false,true}{
  t.Run(fmt.Sprintf("%q/%q/%q/%t",dir,file,rewrite,trimmed),func(t *testing.T){
   base.Ctxt=&obj.Link{Pathname:dir};base.Flag.TrimPath=rewrite;buildcfg.GOROOT="/sdk/go";var m posMap
   want:=snapshot(base.Ctxt,m.makeXPos,trimFilename,file,trimmed)
   link:=&obj.Link{Pathname:dir};p:=ported.NewPosMap(link,rewrite,"/sdk/go")
   got:=snapshot(link,p.XPos,p.TrimFilename,file,trimmed)
   if !reflect.DeepEqual(got,want){t.Fatalf("ported %v\noriginal %v",got,want)}
  })
 }}}}
}
`)
	cmd := exec.Command("go", "test", "-race", "./internal/reference/noder")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOWORK=off", "GOTOOLCHAIN=go1.27.1")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("native position differential: %v\n%s", err, out)
	} else {
		t.Logf("%s native position differential (100 input combinations):\n%s", s.Pin.Tag, out)
	}
}

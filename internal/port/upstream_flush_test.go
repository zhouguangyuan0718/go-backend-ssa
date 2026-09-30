package port

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestPinnedFlush offers a focused offline check of the complete native function flush pipeline.
// The complete upgrade suite also executes these snapshots.
func TestPinnedFlush(t *testing.T) {
	repo := os.Getenv("GO_SSA_SOURCE_REPO")
	if repo == "" {
		t.Skip("set GO_SSA_SOURCE_REPO for pinned function flush differential")
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
			testUpstreamNativeGraph(t, s, r, root, "ssagen", "ssagen_flush")
		})
	}
}

// Read private native symbols without replacing any production pass.
func flushSymbolsObserver() string {
	return `package obj
 func SyncgoInlineCalls(s *LSym) []InlinedCall { return s.Func().Pcln.InlTree.nodes }
 func SyncgoDwarfSymbols(s *LSym) []*LSym { f:=s.Func();return []*LSym{f.dwarfInfoSym,f.dwarfLocSym,f.dwarfRangesSym,f.dwarfAbsFnSym,f.dwarfDebugLinesSym} }
 `
}
func flushSnapshotObserver(code string) string {
	run := `pp.Text.Ctxt.DiagFunc=func(f string,args ...any){panic(fmt.Sprintf(f,args...))};pp.Flush();if pp.Text.Ctxt.Errors!=0{panic("flush failed")}`
	symbols := `appendSym("machine",fn.LSym);for _,jt:=range fi.JumpTables{appendSym("jumpbytes",jt.Sym)}
 for i,sym:=range []*obj.LSym{fi.Pcln.Pcsp,fi.Pcln.Pcfile,fi.Pcln.Pcline,fi.Pcln.Pcinline}{appendSym(fmt.Sprint("pcln",i),sym)}
 for i,sym:=range fi.Pcln.Pcdata{appendSym(fmt.Sprint("pcdata",i),sym)}
 for i,sym:=range fi.Pcln.Funcdata{appendSym(fmt.Sprint("funcdata",i),sym)}
 var used []int;for file:=range fi.Pcln.UsedFiles{used=append(used,int(file))};slices.Sort(used);out=append(out,fmt.Sprint("usedfiles:",used))
 for _,inl:=range obj.SyncgoInlineCalls(fn.LSym){out=append(out,fmt.Sprintf("pcinline:%d:%d:%s",inl.Parent,inl.ParentPC,inl.Func.Name))}
 for i,sym:=range obj.SyncgoDwarfSymbols(fn.LSym){appendSym(fmt.Sprint("dwarf",i),sym)}
 `
	return strings.NewReplacer("ASSEMBLE_RUN", run, "ASSEMBLE_SYMBOL", symbols).Replace(code)
}

func flushFinalObserver() string {
	return `
func SyncgoFlushFinalSnapshot(link *obj.Link) []string {
 var out []string
 text:=append([]*obj.LSym(nil),link.Text...)
 slices.SortFunc(text,func(a,b *obj.LSym)int{if a.Name<b.Name{return -1};if a.Name>b.Name{return 1};return int(a.ABI())-int(b.ABI())})
 for _,fn:=range text{for i,s:=range obj.SyncgoDwarfSymbols(fn){
  if s==nil{out=append(out,fmt.Sprintf("finaldwarf:%s:%d:nil",fn.Name,i));continue}
  out=append(out,fmt.Sprintf("finaldwarf:%s:%d:%s:%d:%x",fn.Name,i,s.Name,s.Size,s.P))
  for _,r:=range s.R{name:="";if r.Sym!=nil{name=r.Sym.Name};out=append(out,fmt.Sprintf("finalreloc:%d:%d:%d:%s:%d",r.Type,r.Off,r.Siz,name,r.Add))}
 }}
 return out
}
`
}

func replaceFlushFixture(t *testing.T, code, old, replacement string) string {
	t.Helper()
	if strings.Count(code, old) != 1 {
		t.Fatalf("flush fixture boundary changed: %q", old)
	}
	return strings.Replace(code, old, replacement, 1)
}

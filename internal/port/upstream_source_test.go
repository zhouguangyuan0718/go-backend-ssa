package port

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPinnedNoderSource(t *testing.T) {
	repo := os.Getenv("GO_SSA_SOURCE_REPO")
	if repo == "" {
		t.Skip("set GO_SSA_SOURCE_REPO for native noder source differential")
	}
	root, e := filepath.Abs("../..")
	if e != nil {
		t.Fatal(e)
	}
	for _, tag := range []string{"go1.27.1", "go1.27.0"} {
		t.Run(tag, func(t *testing.T) {
			pin := Baseline()
			if tag != pin.Tag {
				pin.Tag = tag
				b, e := git(context.Background(), repo, nil, "rev-parse", "refs/tags/"+tag+"^{commit}")
				if e != nil {
					t.Fatal(e)
				}
				pin.Commit = strings.TrimSpace(string(b))
			}
			s, e := OpenSource(context.Background(), repo, pin)
			if e != nil {
				t.Fatal(e)
			}
			r, e := Render(s, Migration(), root)
			if e != nil {
				t.Fatal(e)
			}
			testUpstreamNativeGraph(t, s, r, root, "noder", "noder_source")
		})
	}
}

func sourceSnapshotFixture(t *testing.T, code, dir string, native bool) string {
	t.Helper()
	archive := filepath.Join(dir, "dep.a")
	bind := `link.TargetOS="linux";u.Experiment.RegabiArgs=true;u.Experiment.RegabiWrappers=true;c.BindImports(ObjectConfig{GOOS:"linux",GOARCH:"amd64",GOAMD64:1,Version:"go1.27.1"},"linux",func(p string)(DependencyFile,error){b,e:=os.ReadFile(p);if e!=nil{return nil,e};return &sourceMemory{Reader:bytes.NewReader(b),name:p},nil});c.Flag.Cfg.PackageFile=map[string]string{"example/dep":ARCHIVE};buffers:=map[string]*sourceOutput{};c.OpenSyntaxHTML=func(p string,f int,m fs.FileMode)(io.WriteCloser,error){w:=new(sourceOutput);buffers[p]=w;return w,nil};c.SyntaxWorkingDirectory=func()(string,error){return "/reports",nil}`
	run := `var parsed []*ParsedFile;for i,s:=range sources{f,e:=c.ParseFile(fmt.Sprintf("/src/input%d.go",i),strings.NewReader(s));if e!=nil{t.Fatal(e)};parsed=append(parsed,f)};if _,e:=c.CompileParsedPackage(parsed);e!=nil{t.Fatal(e)}`
	reports := `reports:=map[string]string{};for p,w:=range buffers{if w.closed!=1{t.Fatal("report not closed")};reports[p]=w.String()}`
	imports := `"go-backend-ssa/internal/syntax";"io";"io/fs";`
	extra := `type sourceMemory struct{*bytes.Reader;name string};func(m *sourceMemory)Name()string{return m.name};func(*sourceMemory)Close()error{return nil};type sourceOutput struct{bytes.Buffer;closed int};func(w *sourceOutput)Close()error{w.closed++;return nil}`
	if native {
		bind = `buildcfg.GOAMD64=1;for field:=range reflect.ValueOf(&buildcfg.Experiment.Flags).Elem().NumField(){reflect.ValueOf(&buildcfg.Experiment.Flags).Elem().Field(field).SetBool(false)};buildcfg.Experiment.RegabiArgs=true;buildcfg.Experiment.RegabiWrappers=true;base.Flag.Cfg.PackageFile=map[string]string{"example/dep":ARCHIVE};t.Chdir(t.TempDir())`
		run = `var noders []*noder;for i,s:=range sources{p:=&noder{err:make(chan syntax.Error)};done:=make(chan struct{});go func(){defer close(done);for e:=range p.err{t.Error(e)}}();p.file,_=syntax.Parse(syntax.NewFileBase(fmt.Sprintf("/src/input%d.go",i)),strings.NewReader(s),p.error,p.pragma,syntax.CheckBranches);close(p.err);<-done;if t.Failed(){t.FailNow()};noders=append(noders,p)};unified(posMap{},noders);CloseHTMLWriters()`
		reports = `reports:=map[string]string{};paths,e:=filepath.Glob("*.syntax.html");if e!=nil{t.Fatal(e)};for _,p:=range paths{b,e:=os.ReadFile(p);if e!=nil{t.Fatal(e)};reports[p]=string(b)}`
		imports = `"cmd/compile/internal/syntax";`
		extra = ""
		code = strings.ReplaceAll(code, `"cmd/compile/internal/staticinit";`, "")
	} else {
		bind += `;_=reflect.TypeOf(0);_=filepath.Separator`
	}
	bind = strings.ReplaceAll(bind, "ARCHIVE", fmt.Sprintf("%q", archive))
	return strings.NewReplacer("SOURCE_BIND", bind, "SOURCE_RUN", run, "SOURCE_REPORTS", reports, "SOURCE_IMPORTS", imports).Replace(code) + "\n" + extra
}

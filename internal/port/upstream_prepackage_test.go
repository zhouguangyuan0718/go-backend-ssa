package port

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestPinnedPrepackage(t *testing.T) {
	repo := os.Getenv("GO_SSA_SOURCE_REPO")
	if repo == "" {
		t.Skip("set GO_SSA_SOURCE_REPO for native prepackage differential")
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
			testUpstreamNativeGraph(t, s, r, root, "noder", "noder_prepackage")
		})
	}
}

func prepareBenchmarkDependency(t *testing.T, dir string) {
	t.Helper()
	file := filepath.Join(dir, "testing.go")
	archive := filepath.Join(dir, "testing.a")
	writeTestFile(t, dir, "testing.go", `package testing;type B struct{N int};func(b *B)Loop()bool{b.N--;return b.N>0};func(b *B)More()bool{return b.Loop()}`)
	cmd := exec.Command("go", "tool", "compile", "-p", "testing", "-pack", "-trimpath", dir+"=>/src", "-o", archive, file)
	cmd.Env = append(os.Environ(), "GOWORK=off", "GOTOOLCHAIN=go1.27.1", "GOOS=linux", "GOARCH=amd64", "GOAMD64=v1", "GOEXPERIMENT=none")
	if b, e := cmd.CombinedOutput(); e != nil {
		t.Fatalf("%v\n%s", e, b)
	}
}
func prepackageSnapshotFixture(t *testing.T, code, dir string, native bool) string {
	t.Helper()
	imports := `"go-backend-ssa/internal/bloop";"go-backend-ssa/internal/coverage";"go-backend-ssa/internal/cmd/cov/covcmd";`
	bind := `c.Flag.Cfg.PackageFile["testing"]=ARCHIVE;path:="example/p";if strings.Contains(mode,"main"){path="main"};if strings.Contains(mode,"runtime"){path="runtime"};link.Pkgpath=path;u.LocalPkg=u.NewPkg(path,"")`
	bind += `;irReports:=map[string]*sourceOutput{};graph.OpenAST=func(p string,f int,m fs.FileMode)(io.WriteCloser,error){w:=irReports[p];if w==nil{w=new(sourceOutput);irReports[p]=w};if f&os.O_TRUNC!=0{w.Reset()};return w,nil};graph.ASTWorkingDirectory=func()(string,error){return "/reports",nil}`
	run := `coverage.NewContext(tc).Fixup();bloop.NewContext(tc).Walk(tc.Target)`
	collect := `for p,w:=range irReports{if strings.HasSuffix(p,".ast")&&w.closed!=3||strings.HasSuffix(p,".html")&&w.closed!=1{t.Fatalf("wrong close count %s: %d",p,w.closed)};reports[p]=w.String()}`
	if native {
		imports = `"cmd/compile/internal/bloop";"cmd/compile/internal/coverage";"cmd/internal/cov/covcmd";`
		bind = `base.Flag.Cfg.PackageFile["testing"]=ARCHIVE;path:="example/p";if strings.Contains(mode,"main"){path="main"};if strings.Contains(mode,"runtime"){path="runtime"};base.Ctxt.Pkgpath=path;types.LocalPkg=types.NewPkg(path,"");ir.Pkgs.Coverage=types.NewPkg("go.coverage","runtime/coverage");ir.Pkgs.Coverage.Prefix="runtime/coverage"`
		run = `coverage.Fixup();bloop.Walk(typecheck.Target)`
		collect = `for _,pattern:=range []string{"*.ast","*.html"}{paths,e:=filepath.Glob(pattern);if e!=nil{t.Fatal(e)};for _,p:=range paths{b,e:=os.ReadFile(p);if e!=nil{t.Fatal(e)};reports[p]=string(b)}}`
	}
	bind = strings.ReplaceAll(bind, "ARCHIVE", fmt.Sprintf("%q", filepath.Join(dir, "testing.a")))
	return strings.NewReplacer("PRE_IMPORTS", imports, "PRE_BIND", bind, "PRE_RUN", run, "IR_REPORTS", collect).Replace(code)
}

package port

import (
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestPinnedNoderDependencies(t *testing.T) {
	repo := os.Getenv("GO_SSA_SOURCE_REPO")
	if repo == "" {
		t.Skip("set GO_SSA_SOURCE_REPO for native noder dependencies differential")
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
			testUpstreamNativeGraph(t, s, r, root, "noder", "noder_dependencies")
		})
	}
}
func prepareNativeDependency(t *testing.T, root, dir string) {
	t.Helper()
	b, e := os.ReadFile(filepath.Join(root, "internal/port/testdata/noder_import_fixture.go.txt"))
	if e != nil {
		t.Fatal(e)
	}
	writeTestFile(t, dir, "dep.go", string(b))
	cmd := exec.Command("go", "tool", "compile", "-p", "example/dep", "-pack", "-trimpath", dir+"=>/src", "-o", filepath.Join(dir, "dep.a"), filepath.Join(dir, "dep.go"))
	cmd.Env = append(os.Environ(), "GOWORK=off", "GOTOOLCHAIN=go1.27.1", "GOOS=linux", "GOARCH=amd64", "GOAMD64=v1", "GOEXPERIMENT=none")
	if b, e := cmd.CombinedOutput(); e != nil {
		t.Fatalf("%v\n%s", e, b)
	}
}
func dependencySnapshotFixture(t *testing.T, code, dir string, native bool) string {
	t.Helper()
	archive := filepath.Join(dir, "dep.a")
	setup := `link.Pkgpath="example/p";link.Pathname="/src";link.TargetOS="linux";u.Experiment.RegabiArgs=true;u.Experiment.RegabiWrappers=true;c.BindImports(ObjectConfig{GOOS:"linux",GOARCH:"amd64",GOAMD64:1,Version:"go1.27.1"},runtime.GOOS,func(p string)(DependencyFile,error){data,e:=os.ReadFile(p);if e!=nil{return nil,e};return &snapshotMemory{Reader:bytes.NewReader(data),name:p},nil});c.Flag.Cfg.PackageFile=map[string]string{"example/dep":ARCHIVE};checker:=c.Checker;packages:=c.importPackages`
	read := `c.readImportFile`
	extra := `type snapshotMemory struct{*bytes.Reader;name string};func(s *snapshotMemory)Name()string{return s.name};func(*snapshotMemory)Close()error{return nil}
func readDependencyData(t *testing.T,c *Context,b []byte)(string,error){return c.readExportData(&snapshotMemory{Reader:bytes.NewReader(b),name:"test"})}
`
	call := `readDependencyData(t,c,changed)`
	if native {
		setup = `base.Ctxt.Pkgpath="example/p";base.Ctxt.Pathname="/src";buildcfg.GOOS="linux";buildcfg.GOARCH="amd64";buildcfg.GOAMD64=1;for field:=range reflect.ValueOf(&buildcfg.Experiment.Flags).Elem().NumField(){reflect.ValueOf(&buildcfg.Experiment.Flags).Elem().Field(field).SetBool(false)};buildcfg.Experiment.RegabiArgs=true;buildcfg.Experiment.RegabiWrappers=true;base.Debug.Panic=1;base.Flag.Cfg.PackageFile=map[string]string{"example/dep":ARCHIVE};checker:=types2.NewContext();packages:=map[string]*types2.Package{}`
		read = `readImportFile`
		extra = `func readDependencyData(t *testing.T,b []byte)(string,error){f,e:=os.CreateTemp(t.TempDir(),"archive");if e!=nil{t.Fatal(e)};defer f.Close();if _,e=f.Write(b);e!=nil{t.Fatal(e)};f.Seek(0,0);return readExportData(f)}
`
		call = `readDependencyData(t,changed)`
	} else {
		setup += `;_ = reflect.TypeOf(0)`
	}
	if native {
		code = strings.ReplaceAll(code, "DEPS_BASE_EXTRA", "")
		extra += `func readPortableData(t *testing.T,b []byte,off,n int64)(string,error){f,e:=os.CreateTemp(t.TempDir(),"portable");if e!=nil{t.Fatal(e)};defer f.Close();if _,e=f.Write(b);e!=nil{t.Fatal(e)};return base.SyncgoPortableMapFile(f,off,n)}
`
	} else {
		code = strings.ReplaceAll(code, "DEPS_BASE_EXTRA", `"go-backend-ssa/internal/base";`)
		extra += `func readPortableData(t *testing.T,b []byte,off,n int64)(string,error){return base.MapFile(bytes.NewReader(b),off,n)}
`
	}
	setup = strings.ReplaceAll(setup, "ARCHIVE", `"`+archive+`"`)
	code = strings.NewReplacer("DEPENDENCY_BIND", setup, "READ_IMPORT", read, "READ_DEPENDENCY_DATA", call, "ARCHIVE", `"`+archive+`"`).Replace(code)
	return code + "\n" + extra
}

// The observer preserves the complete upstream portable function independently
// of the stream projection and only gives it a distinct test-only name.
func nativePortableMapObserver(t *testing.T, s *Source) string {
	t.Helper()
	b, e := s.Read("src/cmd/compile/internal/base/mapfile_read.go")
	if e != nil {
		t.Fatal(e)
	}
	fs := token.NewFileSet()
	f, e := parser.ParseFile(fs, "mapfile_read.go", b, 0)
	if e != nil {
		t.Fatal(e)
	}
	var fn *ast.FuncDecl
	for _, d := range f.Decls {
		if d, ok := d.(*ast.FuncDecl); ok && d.Name.Name == "MapFile" {
			fn = d
		}
	}
	if fn == nil {
		t.Fatal("portable native MapFile missing")
	}
	fn.Name.Name = "SyncgoPortableMapFile"
	return "package base\nimport(\"io\";\"os\")\n" + nodeText(fs, fn)
}

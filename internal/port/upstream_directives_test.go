package port

import (
	"bytes"
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

func TestPinnedNoderDirectives(t *testing.T) {
	repo := os.Getenv("GO_SSA_SOURCE_REPO")
	if repo == "" {
		t.Skip("set GO_SSA_SOURCE_REPO for native directive differential")
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
			testUpstreamNoderDirectives(t, s, r, root)
		})
	}
}

// Reference bodies are selected directly from the pin with no Context, Owner,
// State, or Call rewrites. Only import paths and explicit test globals change.
func testUpstreamNoderDirectives(t *testing.T, s *Source, r *Rendered, root string) {
	t.Helper()
	dir := t.TempDir()
	for name, data := range r.Files {
		writeTestFile(t, dir, name, string(data))
	}
	for _, name := range Migration().SupportFiles {
		b, e := os.ReadFile(filepath.Join(root, name))
		if e != nil {
			t.Fatal(e)
		}
		writeTestFile(t, dir, name, string(b))
	}
	for _, u := range Migration().Units {
		if u.Context != nil && u.Context.Adapter != "" {
			b, e := os.ReadFile(filepath.Join(root, u.adapterPath()))
			if e != nil {
				t.Fatal(e)
			}
			writeTestFile(t, dir, u.adapterPath(), string(b))
		}
	}
	mapping := Migration().importMap(s.Pin.TargetModule)
	mapping["cmd/compile/internal/base"] = "go-backend-ssa/internal/reference/base"
	mapping["cmd/internal/objabi"] = "go-backend-ssa/internal/reference/objabi"
	mapping["internal/buildcfg"] = "go-backend-ssa/internal/reference/buildcfg"
	for _, u := range []Unit{
		{Source: "src/cmd/compile/internal/noder/noder.go", Functions: []string{"trimFilename", "isCgoGeneratedFile", "safeArg", "parseGoEmbed"}, Declarations: []string{"noder", "noder.error", "noder.pragma", "noder.checkUnusedDuringParse", "linkname", "pragmas", "pragmas.Nointerface", "WasmImport", "WasmExport", "pragmaPos", "pragmaEmbed"}},
		{Source: "src/cmd/compile/internal/noder/lex.go", WholeFile: true},
		{Source: "src/cmd/internal/objabi/line.go", Functions: []string{"AbsFile", "ApplyRewrites", "applyRewrite", "hasPathPrefix"}},
	} {
		b, e := s.Read(u.Source)
		if e != nil {
			t.Fatal(e)
		}
		fs := token.NewFileSet()
		f, e := selectFile(fs, u.Source, b, u)
		if e != nil {
			t.Fatal(e)
		}
		if strings.HasSuffix(u.Source, "/noder/noder.go") {
			original, err := parser.ParseFile(fs, u.Source, b, 0)
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for _, decl := range original.Decls {
				g, ok := decl.(*ast.GenDecl)
				if !ok || g.Tok != token.VAR {
					continue
				}
				for _, spec := range g.Specs {
					v := spec.(*ast.ValueSpec)
					for _, name := range v.Names {
						if name.Name == "allowedStdPragmas" {
							f.Decls = append(f.Decls, &ast.GenDecl{Tok: token.VAR, Specs: []ast.Spec{v}})
							found = true
						}
					}
				}
			}
			if !found {
				t.Fatal("native pragma table missing")
			}
		}
		pruneImports(f)
		code := nodeText(fs, f)
		for from, to := range mapping {
			code = strings.ReplaceAll(code, `"`+from+`"`, `"`+to+`"`)
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
var GOARCH,GOOS,GOROOT string
var Experiment struct{FieldTrack bool}
`)
	// This pure helper is already independently migrated; pragmas use its exact ABI prefix encoding.
	writeTestFile(t, dir, "internal/reference/objabi/prefix.go", `package objabi
import original "go-backend-ssa/internal/cmd/objabi"
func PathToPrefix(s string)string{return original.PathToPrefix(s)}
`)
	fixture, e := os.ReadFile("testdata/noder_directives_snapshot.go.txt")
	if e != nil {
		t.Fatal(e)
	}
	native := strings.Replace(string(fixture), `"testing"`, `"testing";"go-backend-ssa/internal/reference/base";"go-backend-ssa/internal/reference/buildcfg";"go-backend-ssa/internal/cmd/obj"`, 1) + `
func parseDirectiveSnapshot(t *testing.T,c directiveConfig,name,source string)(*noder,[]string){
 base.Ctxt=&obj.Link{Pkgpath:"example/p",Pathname:"/src"};base.Flag.Std=c.Std;base.Flag.CompilingRuntime=c.Runtime
 buildcfg.GOARCH=c.Arch;buildcfg.GOOS=c.OS;buildcfg.Experiment.FieldTrack=c.FieldTrack
 p:=&noder{err:make(chan syntax.Error)};var errs []string;done:=make(chan struct{})
 go func(){defer close(done);for e:=range p.err{errs=append(errs,e.Error())}}()
 p.file,_=syntax.Parse(syntax.NewFileBase(name),strings.NewReader(source),p.error,p.pragma,syntax.CheckBranches)
 close(p.err);<-done;return p,errs
}
`
	ported := strings.Replace(string(fixture), `"testing"`, `"testing";"go-backend-ssa/internal/cmd/obj";"go-backend-ssa/internal/cmd/sys";"go-backend-ssa/internal/ir";"go-backend-ssa/internal/typecheck";"go-backend-ssa/internal/types";"go-backend-ssa/internal/types2"`, 1) + `
func parseDirectiveSnapshot(t *testing.T,cfg directiveConfig,name,source string)(*noder,[]string){
 link:=&obj.Link{Pkgpath:"example/p",Pathname:"/src",Arch:&obj.LinkArch{Arch:&sys.Arch{Name:cfg.Arch,PtrSize:8,RegSize:8}}}
 u:=types.NewUniverseWithPositions(8,8,1<<50,&link.PosTable);u.LocalPkg=u.NewPkg("example/p","p")
 tc:=typecheck.NewContext(ir.NewContext(u,link));tc.InitUniverse()
 c,e:=NewContext(types2.NewContext(),tc);if e!=nil{t.Fatal(e)};c.GOOS=cfg.OS;c.Flag.Std=cfg.Std;c.Flag.CompilingRuntime=cfg.Runtime;c.Types.Experiment.FieldTrack=cfg.FieldTrack
 p,e:=c.ParseFile(name,strings.NewReader(source));if e!=nil{var errs []string;for _,x:=range e.(ParseErrors){errs=append(errs,x.Error())};return nil,errs};return p.native,nil
}
`
	writeTestFile(t, dir, "internal/reference/noder/directives_test.go", native)
	writeTestFile(t, dir, "internal/noder/adapter_directives_snapshot_test.go", ported)
	writeTestFile(t, dir, "go.mod", "module go-backend-ssa\n\ngo 1.27.1\n")
	var snapshots [][]byte
	for _, pkg := range []string{"./internal/reference/noder", "./internal/noder"} {
		path := filepath.Join(t.TempDir(), "snapshot.json")
		cmd := exec.Command("go", "test", "-race", pkg, "-run", "^TestDirectiveSnapshot$", "-count=1", "-v")
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GOWORK=off", "GOTOOLCHAIN=go1.27.1", "DIRECTIVE_SNAPSHOT="+path)
		out, e := cmd.CombinedOutput()
		if e != nil {
			t.Fatalf("%s: %v\n%s", pkg, e, out)
		}
		t.Logf("%s: %s", pkg, out)
		b, e := os.ReadFile(path)
		if e != nil {
			t.Fatal(e)
		}
		snapshots = append(snapshots, b)
	}
	if !bytes.Equal(snapshots[0], snapshots[1]) {
		t.Fatalf("native and ported directive snapshots differ\nnative: %s\nported: %s", snapshots[0], snapshots[1])
	}
}

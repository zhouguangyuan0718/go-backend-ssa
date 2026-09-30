package compiler_test

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"go-backend-ssa/compiler"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
)

// The official compiler writes its own Built-phase dumps. The object inspector
// uses the official goobj.Reader through a temporary overlay; neither oracle
// changes compiler algorithms or uses the library's migration transform.
func TestSourceBuildOfficialOracle(t *testing.T) { testSourceBuildOfficialOracle(t, compiler.Built) }
func TestSourceEarlyOfficialOracle(t *testing.T) {
	testSourceBuildOfficialOracle(t, compiler.EarlyOptimized)
}

func TestARM64BuildOfficialOracle(t *testing.T) {
	testSourceBuildOfficialOracle(t, compiler.Built, "arm64")
}
func TestARM64EarlyOfficialOracle(t *testing.T) {
	testSourceBuildOfficialOracle(t, compiler.EarlyOptimized, "arm64")
}

func testSourceBuildOfficialOracle(t *testing.T, stage compiler.Stage, architectures ...string) {
	arch := "amd64"
	if len(architectures) > 0 {
		arch = architectures[0]
	}
	if os.Getenv("GO_SSA_NATIVE_EXECUTION") == "" {
		t.Skip("set GO_SSA_NATIVE_EXECUTION for official Built dumps and object data")
	}
	dir := t.TempDir()
	run := func(env []string, args ...string) []byte {
		t.Helper()
		cmd := exec.Command("go", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GOTOOLCHAIN=go1.27.1", "GOWORK=off", "GO111MODULE=off", "GOFLAGS=", "GOOS=linux", "GOARCH="+arch, "GOAMD64=v1", "GOARM64=v8.0", "GOEXPERIMENT=", "CGO_ENABLED=0", "GOPROXY=off", "GOSUMDB=off", "GOSSADIR="+dir, "GOSSAFUNC=", "GOCOMPILEDEBUG=")
		cmd.Env = append(cmd.Env, env...)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("go %v: %v\n%s", args, err, out)
		}
		return out
	}
	root := strings.TrimSpace(string(run(nil, "env", "GOROOT")))
	inspector := filepath.Join(dir, "inspect.go")
	write := func(path string, data []byte) {
		t.Helper()
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	write(inspector, []byte(officialObjectInspector))
	virtual := filepath.Join(root, "src/cmd/compile/syncgo_object_inspector.go")
	overlay, _ := json.Marshal(map[string]any{"Replace": map[string]string{virtual: inspector}})
	overlayFile := filepath.Join(dir, "overlay.json")
	write(overlayFile, overlay)
	inspectorBinary := filepath.Join(dir, "inspect")
	buildInspector := exec.Command(filepath.Join(root, "bin/go"), "build", "-overlay", overlayFile, "-o", inspectorBinary, virtual)
	buildInspector.Dir = dir
	buildInspector.Env = append(os.Environ(), "GOROOT="+root, "GOTOOLCHAIN=local", "GOMODCACHE="+filepath.Join(dir, "modcache"), "GOWORK=off", "GO111MODULE=off", "GOFLAGS=", "GOOS="+runtime.GOOS, "GOARCH="+runtime.GOARCH, "CGO_ENABLED=0", "GOPROXY=off", "GOSUMDB=off")
	if out, err := buildInspector.CombinedOutput(); err != nil {
		t.Fatalf("build official inspector: %v\n%s", err, out)
	}
	source := `package p
 type V struct{ A int; B string }
 func(v V)Value()int{return v.A}
 var M=map[string]int{"hello":3}
 var I any=V{4,"literal"}
 func init(){M["hello"]++}
 func Add[T ~int](a,b T)T{return a+b}
 func F(n int)int{s:=0;for i:=range n{if i&1==0{s+=i}else{s-=i}};return Add(s,1)}
 func Closure(n int)func()int{return func()int{return n+M["hello"]}}
 func Defer(p *int){defer func(){*p+=2}();defer func(){*p+=3}()}
 `
	file := filepath.Join(dir, "p.go")
	write(file, []byte(source))
	modes := []string{"default", "noinline", "noopt", "amd64v3"}
	if arch == "arm64" {
		modes[3] = "arm64v81"
	}
	for _, mode := range modes {
		t.Run(mode, func(t *testing.T) {
			req := request("p", source)
			if arch == "arm64" {
				req.Target = compiler.Target{GOOS: "linux", GOARCH: arch, GOARM64: "v8.0"}
			}
			req.Options.Stage = stage
			req.Files[0].Name = file
			req.Options.Complete = true
			args := []string{"tool", "compile", "-p", "p", "-complete", "-dwarf=false"}
			if mode == "noinline" {
				req.Options.DisableInlining = true
				args = append(args, "-l")
			}
			if mode == "noopt" {
				req.Options.DisableOptimization = true
				args = append(args, "-N")
			}
			if mode == "amd64v3" {
				req.Target.GOAMD64 = 3
			}
			if mode == "arm64v81" {
				req.Target.GOARM64 = "v8.1"
			}
			p, err := compiler.Build(context.Background(), req)
			if err != nil {
				t.Fatal(err)
			}
			phase := "build"
			if stage == compiler.EarlyOptimized {
				phase = "early_fuse"
				if req.Options.DisableOptimization {
					phase = "middle_opt"
				}
			}
			var dumpFlags []string
			names := map[string]bool{}
			for _, f := range p.Functions {
				if names[f.Name] {
					t.Fatal("ambiguous dump name", f.Name)
				}
				names[f.Name] = true
				dumpFlags = append(dumpFlags, "ssa/"+phase+"/dump="+f.Name)
			}
			archive := filepath.Join(dir, mode+".a")
			args = append(args, "-d="+strings.Join(dumpFlags, ","), "-o", archive, file)
			cpuEnv := []string{fmt.Sprintf("GOAMD64=v%d", req.Target.GOAMD64)}
			if arch == "arm64" {
				cpuEnv = []string{"GOARM64=" + p.Metadata.Target.GOARM64}
			}
			run(cpuEnv, args...)
			for _, f := range p.Functions {
				name := strings.NewReplacer(" ", "_", "/", "_", ":", "_").Replace(f.Name) + "_01__" + phase + ".dump"
				expected, err := os.ReadFile(filepath.Join(dir, name))
				if err != nil {
					t.Fatal(err)
				}
				actual := liveDump(f.String())
				if string(expected) != actual {
					t.Fatalf("%s dump differs for %s\nlibrary:\n%s\nofficial:\n%s", stage, f.Name, actual, expected)
				}
				if err := os.Remove(filepath.Join(dir, name)); err != nil {
					t.Fatal(err)
				}
			}
			object := run(nil, "tool", "pack", "p", archive, "_go_.o")
			objectPath := filepath.Join(dir, mode+".o")
			write(objectPath, object)
			raw, err := exec.Command(inspectorBinary, objectPath).CombinedOutput()
			if err != nil {
				t.Fatalf("official object inspector: %v\n%s", err, raw)
			}
			var symbols []oracleSymbol
			if err := json.Unmarshal(raw, &symbols); err != nil {
				t.Fatal(err, string(raw))
			}
			byName := map[string][]oracleSymbol{}
			var nativeFuncs []string
			for _, s := range symbols {
				byName[s.Name] = append(byName[s.Name], s)
				if s.Text {
					nativeFuncs = append(nativeFuncs, s.Name)
				}
			}
			var builtFuncs []string
			for _, f := range p.Functions {
				builtFuncs = append(builtFuncs, f.Frontend().Func().LSym.Name)
			}
			slices.Sort(nativeFuncs)
			slices.Sort(builtFuncs)
			if !slices.Equal(nativeFuncs, builtFuncs) {
				t.Fatal("package function set differs", nativeFuncs, builtFuncs)
			}
			for _, s := range p.Globals {
				var actualRelocs []oracleReloc
				for _, r := range s.R {
					name := ""
					if r.Sym != nil {
						name = r.Sym.Name
					}
					actualRelocs = append(actualRelocs, oracleReloc{r.Off, r.Siz, uint16(r.Type), r.Add, name})
				}
				order := func(a, b oracleReloc) int {
					if x := cmp.Compare(a.Off, b.Off); x != 0 {
						return x
					}
					if x := cmp.Compare(a.Size, b.Size); x != 0 {
						return x
					}
					if x := cmp.Compare(a.Kind, b.Kind); x != 0 {
						return x
					}
					if x := cmp.Compare(a.Add, b.Add); x != 0 {
						return x
					}
					return cmp.Compare(a.Name, b.Name)
				}
				// Native object emission sorts relocations; Built retains their insertion order.
				slices.SortFunc(actualRelocs, order)
				matched := -1
				for j, native := range byName[s.Name] {
					slices.SortFunc(native.Relocs, order)
					if native.Size == s.Size && native.Align == int32(s.Align) && native.Kind == uint8(s.Type) && bytes.Equal(native.Data, s.P) && slices.Equal(actualRelocs, native.Relocs) {
						matched = j
						break
					}
				}
				if matched < 0 {
					t.Fatalf("global differs %q: native=%+v library=%d/%d/%d/%x/%+v", s.Name, byName[s.Name], s.Size, s.Align, s.Type, s.P, actualRelocs)
				}
				candidates := byName[s.Name]
				byName[s.Name] = append(candidates[:matched], candidates[matched+1:]...)

			}
			t.Logf("%d %s graphs, complete function set and %d globals with relocations match official compiler", len(p.Functions), stage, len(p.Globals))
		})
	}
}

// Official dump files omit dead values/blocks. Keep native IDs, operations,
// arguments, Aux values, source positions and named values exactly as printed.
func liveDump(s string) string {
	var out strings.Builder
	deadBlock := false
	for _, line := range strings.SplitAfter(s, "\n") {
		text := strings.TrimSuffix(line, "\n")
		if strings.HasPrefix(text, "  b") {
			deadBlock = strings.HasSuffix(text, " DEAD")
		}
		if strings.HasPrefix(text, "name ") {
			deadBlock = false
		}
		if deadBlock || strings.HasSuffix(text, " DEAD") {
			continue
		}
		out.WriteString(line)
	}
	return out.String()
}

type oracleReloc struct {
	Off  int32
	Size uint8
	Kind uint16
	Add  int64
	Name string
}
type oracleSymbol struct {
	Name   string
	Size   int64
	Align  int32
	Kind   uint8
	Text   bool
	Data   []byte
	Relocs []oracleReloc
}

const officialObjectInspector = `package main
import("bytes";"encoding/json";"fmt";"os";"cmd/internal/goobj";"cmd/internal/objabi")
type Reloc struct{Off int32;Size uint8;Kind uint16;Add int64;Name string}
type Symbol struct{Name string;Size int64;Align int32;Kind uint8;Text bool;Data []byte;Relocs []Reloc}
func main(){data,e:=os.ReadFile(os.Args[1]);if e!=nil{panic(e)};off:=bytes.Index(data,[]byte(goobj.Magic));if off<0{panic("missing object payload")};r:=goobj.NewReaderFromBytes(data[off:],true);if r==nil{panic("bad object")}
 names:=map[goobj.SymRef]string{};base:=0
 for _,g:=range []struct{pkg uint32;n int}{{goobj.PkgIdxSelf,r.NSym()},{goobj.PkgIdxHashed64,r.NHashed64def()},{goobj.PkgIdxHashed,r.NHasheddef()},{goobj.PkgIdxNone,r.NNonpkgdef()+r.NNonpkgref()}}{for j:=0;j<g.n;j++{names[goobj.SymRef{PkgIdx:g.pkg,SymIdx:uint32(j)}]=r.Sym(uint32(base+j)).Name(r)};base+=g.n}
 for i:=0;i<r.NRefName();i++{ref:=r.RefName(i);names[ref.Sym()]=ref.Name(r)}
 var out []Symbol
 for i:=0;i<r.NSym()+r.NHashed64def()+r.NHasheddef()+r.NNonpkgdef();i++{s:=r.Sym(uint32(i));row:=Symbol{Name:s.Name(r),Size:int64(s.Siz()),Align:int32(s.Align()),Kind:s.Type(),Text:objabi.SymKind(s.Type()).IsText(),Data:r.Data(uint32(i))};for _,rel:=range r.Relocs(uint32(i)){ref:=rel.Sym();name:=names[ref];if ref.PkgIdx==goobj.PkgIdxBuiltin{name,_=goobj.BuiltinName(int(ref.SymIdx))};if name==""&&ref.PkgIdx!=0{panic(fmt.Sprintf("unknown reference %+v",ref))};row.Relocs=append(row.Relocs,Reloc{rel.Off(),rel.Siz(),rel.Type(),rel.Add(),name})};out=append(out,row)}
 if e:=json.NewEncoder(os.Stdout).Encode(out);e!=nil{panic(e)}}
`

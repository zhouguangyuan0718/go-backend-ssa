package port

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Run identical graph operations against migrated code and the original native
// compiler. The reference uses immutable Git blobs through a temporary Go build
// overlay; neither GOROOT nor the user's upstream checkout is modified.
func testUpstreamIR(t *testing.T, s *Source, r *Rendered, root string) {
	testUpstreamNativeGraph(t, s, r, root, "ir")
	testUpstreamNativeGraph(t, s, r, root, "typecheck")
	testUpstreamNativeGraph(t, s, r, root, "noder")
	testUpstreamNativeGraph(t, s, r, root, "noder", "noder_dict")
	testUpstreamNativeGraph(t, s, r, root, "dwarfgen")
	testUpstreamNativeGraph(t, s, r, root, "objw")
	testUpstreamNativeGraph(t, s, r, root, "staticdata")
	testUpstreamNativeGraph(t, s, r, root, "rttype")
	testUpstreamNativeGraph(t, s, r, root, "reflectdata")
	testUpstreamNativeGraph(t, s, r, root, "reflectdata", "reflectdata_descriptors")
}
func testUpstreamNativeGraph(t *testing.T, s *Source, r *Rendered, root, pkgName string, snapshots ...string) {
	t.Helper()
	snapshotName := pkgName
	if len(snapshots) > 0 {
		snapshotName = snapshots[0]
	}
	template, err := os.ReadFile(filepath.Join(root, "internal/port/testdata/"+snapshotName+"_snapshot.go.txt"))
	if err != nil {
		t.Fatal(err)
	}
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
		if u.Context != nil {
			data, err := os.ReadFile(filepath.Join(root, u.adapterPath()))
			if err != nil {
				t.Fatal(err)
			}
			writeTestFile(t, dir, u.adapterPath(), string(data))
		}
	}
	writeTestFile(t, dir, "go.mod", "module go-backend-ssa\n\ngo 1.27.1\n")
	setup := `link:=&obj.Link{};u:=types.NewUniverseWithPositions(8,8,1<<50,&link.PosTable);u.LocalPkg=u.NewPkg("example/p","p");c:=NewContext(u,link)`
	position := "c.Types.Pos"
	if pkgName == "typecheck" {
		setup = strings.Replace(setup, "c:=NewContext(u,link)", "graph:=ir.NewContext(u,link);c:=NewContext(graph)", 1)
		position = "u.Pos"
	}
	if pkgName == "noder" {
		setup = `link:=&obj.Link{};u:=types.NewUniverseWithPositions(8,8,1<<50,&link.PosTable);u.LocalPkg=u.NewPkg("example/p","p");u.BindLink(link);c:=&Context{Base:u.LinkContext(),Ctxt:link,Types:u,GOROOT:"/sdk/go"}`
	}
	if pkgName == "dwarfgen" {
		setup = `link:=&obj.Link{};u:=types.NewUniverseWithPositions(8,8,1<<50,&link.PosTable);graph:=ir.NewContext(u,link)`
	}
	if pkgName == "objw" {
		setup = `link:=&obj.Link{Arch:&obj.LinkArch{Arch:&sys.Arch{Name:target.arch,PtrSize:target.size,RegSize:target.size,ByteOrder:target.order}}};u:=types.NewUniverseWithPositions(target.size,target.size,1<<50,&link.PosTable);c:=NewContext(u,link,target.os)`
	}
	if pkgName == "rttype" {
		setup = `link:=&obj.Link{Arch:&obj.LinkArch{Arch:&sys.Arch{Name:target.arch,PtrSize:target.size,RegSize:target.size,ByteOrder:target.order}}};u:=types.NewUniverseWithPositions(target.size,target.size,1<<50,&link.PosTable);typecheck.NewContext(ir.NewContext(u,link)).InitUniverse();c:=NewContext(objw.NewContext(u,link,"linux"))`
	}
	if pkgName == "reflectdata" {
		position = "u.Pos"
		setup = `link:=&obj.Link{Arch:&obj.LinkArch{Arch:&sys.Arch{Name:target.arch,PtrSize:target.size,RegSize:target.size,ByteOrder:target.order}}};u:=types.NewUniverseWithPositions(target.size,target.size,1<<50,&link.PosTable);u.LocalPkg=u.NewPkg("example/p","p");graph:=ir.NewContext(u,link);tc:=typecheck.NewContext(graph);tc.InitUniverse();rt:=rttype.NewContext(objw.NewContext(u,link,"linux"));rt.Init();c:=NewContext(tc,rt,staticdata.NewContext(tc.IR,rt.Writer))`
	}
	if pkgName == "staticdata" {
		setup = `link:=&obj.Link{Arch:&obj.LinkArch{Arch:&sys.Arch{Name:target.arch,PtrSize:target.size,RegSize:target.size,ByteOrder:target.order}}};u:=types.NewUniverseWithPositions(target.size,target.size,1<<50,&link.PosTable);u.LocalPkg=u.NewPkg("example/p","p");graph:=ir.NewContext(u,link);typecheck.NewContext(graph).InitUniverse();c:=NewContext(graph,objw.NewContext(u,link,"linux"))`
	}
	if snapshotName == "noder_dict" {
		setup = `link:=&obj.Link{Arch:&obj.LinkArch{Arch:&sys.Arch{Name:target.arch,PtrSize:target.size,RegSize:target.size,ByteOrder:target.order}}};u:=types.NewUniverseWithPositions(target.size,target.size,1<<50,&link.PosTable);u.LocalPkg=u.NewPkg("example/p","p");graph:=ir.NewContext(u,link);tc:=typecheck.NewContext(graph);tc.InitUniverse();c,err:=NewContext(types2.NewContext(),tc);if err!=nil{t.Fatal(err)}`
		position = "u.Pos"
	}
	ported := strings.NewReplacer("IMPORT_TYPES2", "go-backend-ssa/internal/types2", "DICT_CONTEXT", "c,", "DICT_ONLY", "c", "ND_DEBUG", "c.Typecheck.IR.Debug", "RD_STATIC", `"go-backend-ssa/internal/staticdata";`, "RD_FLAG", "c.Typecheck.IR.Base.Flag", "SD_FLAG", "c.IR.Base.Flag", "IMPORT_TYPEBITS", "go-backend-ssa/internal/typebits", "IMPORT_RTTYPE", "go-backend-ssa/internal/rttype", "RD_OBJW", `"go-backend-ssa/internal/objw";`, "EXPERIMENT", "u.Experiment", "TC.", "c.Typecheck.", "IMPORT_OBJW", "go-backend-ssa/internal/objw", "IMPORT_TYPECHECK", "go-backend-ssa/internal/typecheck", "RT_IR_OBJW", `"go-backend-ssa/internal/ir";"go-backend-ssa/internal/objw";`, "IMPORT_SYS", "go-backend-ssa/internal/cmd/sys", "IMPORT_OBJABI", "go-backend-ssa/internal/cmd/objabi", "IMPORT_BITVEC", "go-backend-ssa/internal/bitvec", "FIPSHASH", "link.SetFIPSDebugHash", "IMPORT_PKGBITS", "go-backend-ssa/internal/go/pkgbits", "IMPORT_BUILDCFG", "", "MAKEMARKER", "NewScopeMarker(graph.Base)", "IMPORT_IR", "go-backend-ssa/internal/ir", "GRAPH.", "graph.", "IMPORT_TYPES", "go-backend-ssa/internal/types", "IMPORT_OBJ", "go-backend-ssa/internal/cmd/obj", "IMPORT_SRC", "go-backend-ssa/internal/cmd/src", "IMPORT_BASE", "", "SETUP", setup, "UP.", "c.", "TY.", "u.", "LINK", "link", "POS", position).Replace(string(template))
	writeTestFile(t, dir, "internal/"+pkgName+"/adapter_snapshot_test.go", ported)
	snapshotTarget := ""
	run := func(args ...string) []byte {
		cmd := exec.Command("go", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GOWORK=off", "GOTOOLCHAIN=go1.27.1", "SYNCGO_TARGET="+snapshotTarget)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("%v: %v\n%s", args, err, out)
		}
		return out
	}
	goroot := strings.TrimSpace(string(run("env", "GOROOT")))
	overlay := map[string]string{}
	for _, pkg := range []string{"cmd/compile/internal/ir", "cmd/compile/internal/types", "cmd/compile/internal/base", "cmd/internal/obj", "cmd/compile/internal/typecheck", "cmd/compile/internal/noder", "cmd/compile/internal/dwarfgen", "cmd/compile/internal/objw", "cmd/compile/internal/bitvec", "cmd/internal/objabi", "internal/bisect", "internal/abi", "cmd/compile/internal/rttype", "cmd/compile/internal/reflectdata", "cmd/compile/internal/typebits", "cmd/compile/internal/staticdata", "cmd/internal/hash", "cmd/compile/internal/compare"} {
		for _, name := range s.PackageFiles(pkg) {
			data, err := s.Read(name)
			if err != nil {
				t.Fatal(err)
			}
			dest := filepath.Join("reference", name)
			writeTestFile(t, dir, dest, string(data))
			overlay[filepath.Join(goroot, name)] = filepath.Join(dir, dest)
		}
	}
	nativeSetup := `base.Ctxt=&obj.Link{};types.PtrSize=8;types.RegSize=8;types.MaxWidth=1<<50;types.BuiltinPkg=types.NewPkg("go.builtin","");types.LocalPkg=types.NewPkg("example/p","p");types.UnsafePkg=types.NewPkg("unsafe","unsafe")`
	if pkgName == "typecheck" {
		nativeSetup += `;Target=&ir.Package{};ir.Pkgs.Runtime=types.NewPkg("go.runtime","runtime");ir.Pkgs.Runtime.Prefix="runtime";ir.Pkgs.Coverage=types.NewPkg("go.coverage","runtime/coverage");ir.Pkgs.Coverage.Prefix="runtime/coverage"`
	}
	if pkgName == "noder" {
		nativeSetup += `;buildcfg.GOROOT="/sdk/go"`
	}
	if pkgName == "objw" {
		nativeSetup = `base.Ctxt=&obj.Link{Arch:&obj.LinkArch{Arch:&sys.Arch{Name:target.arch,PtrSize:target.size,RegSize:target.size,ByteOrder:target.order}}};types.PtrSize=target.size;types.RegSize=target.size;buildcfg.GOOS=target.os;buildcfg.GOARCH=target.arch;obj.SetFIPSDebugHash("")`
	}
	if pkgName == "rttype" || pkgName == "reflectdata" || pkgName == "staticdata" {
		nativeSetup = `base.Ctxt=&obj.Link{Arch:&obj.LinkArch{Arch:&sys.Arch{Name:target.arch,PtrSize:target.size,RegSize:target.size,ByteOrder:target.order}}};types.PtrSize=target.size;types.RegSize=target.size;types.MaxWidth=1<<50;types.BuiltinPkg=types.NewPkg("go.builtin","");types.LocalPkg=types.NewPkg("example/p","p");types.UnsafePkg=types.NewPkg("unsafe","unsafe");typecheck.InitUniverse()`
	}
	if pkgName == "reflectdata" {
		nativeSetup += `;ir.Pkgs.InternalMaps=types.NewPkg("go.internal/runtime/maps","internal/runtime/maps");ir.Pkgs.InternalMaps.Prefix="internal/runtime/maps";ir.Pkgs.Runtime=types.NewPkg("go.runtime","runtime");ir.Pkgs.Runtime.Prefix="runtime";typecheck.Target=&ir.Package{};ir.Pkgs.Itab=types.NewPkg("go.itab","go.itab");ir.Pkgs.Itab.Prefix="go:itab";base.AutogeneratedPos=base.Ctxt.PosTable.XPos(src.MakePos(src.NewFileBase("<autogenerated>","<autogenerated>"),1,0));rttype.Init()`
	}
	if snapshotName == "noder_dict" {
		nativeSetup = `base.Ctxt=&obj.Link{Arch:&obj.LinkArch{Arch:&sys.Arch{Name:target.arch,PtrSize:target.size,RegSize:target.size,ByteOrder:target.order}}};types.PtrSize=target.size;types.RegSize=target.size;types.MaxWidth=1<<50;types.BuiltinPkg=types.NewPkg("go.builtin","");types.LocalPkg=types.NewPkg("example/p","p");types.UnsafePkg=types.NewPkg("unsafe","unsafe");typecheck.InitUniverse()`
	}
	native := strings.NewReplacer("IMPORT_TYPES2", "cmd/compile/internal/types2", "DICT_CONTEXT", "", "DICT_ONLY", "", "ND_DEBUG", "base.Debug", "RD_STATIC", "", "RD_FLAG", "base.Flag", "SD_FLAG", "base.Flag", "IMPORT_TYPEBITS", "cmd/compile/internal/typebits", "IMPORT_RTTYPE", "cmd/compile/internal/rttype", "RD_OBJW", "", "EXPERIMENT", "buildcfg.Experiment", "TC.", "typecheck.", "IMPORT_OBJW", "cmd/compile/internal/objw", "IMPORT_TYPECHECK", "cmd/compile/internal/typecheck", "RT_IR_OBJW", "", "IMPORT_SYS", "cmd/internal/sys", "IMPORT_OBJABI", "cmd/internal/objabi", "IMPORT_BITVEC", "cmd/compile/internal/bitvec", "FIPSHASH", "obj.SetFIPSDebugHash", "IMPORT_PKGBITS", "internal/pkgbits", "IMPORT_BUILDCFG", `;"internal/buildcfg"`, "MAKEMARKER", "ScopeMarker{}", "IMPORT_IR", "cmd/compile/internal/ir", "GRAPH.", "ir.", "IMPORT_TYPES", "cmd/compile/internal/types", "IMPORT_OBJ", "cmd/internal/obj", "IMPORT_SRC", "cmd/internal/src", "IMPORT_BASE", `"cmd/compile/internal/base"`, "SETUP", nativeSetup, "UP.", "", "TY.", "types.", "LINK", "base.Ctxt", "POS", "base.Pos").Replace(string(template))
	writeTestFile(t, dir, "reference/"+pkgName+"_snapshot_test.go", native)
	overlay[filepath.Join(goroot, "src/cmd/compile/internal/"+pkgName+"/syncgo_snapshot_test.go")] = filepath.Join(dir, "reference/"+pkgName+"_snapshot_test.go")
	data, err := json.Marshal(struct{ Replace map[string]string }{overlay})
	if err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, dir, "overlay.json", string(data))
	extract := func(out []byte) []byte {
		for _, line := range bytes.Split(out, []byte{'\n'}) {
			if bytes.HasPrefix(line, []byte("SYNCGO_IR=")) {
				return bytes.TrimPrefix(line, []byte("SYNCGO_IR="))
			}
		}
		t.Fatalf("snapshot missing: %s", out)
		return nil
	}
	// A downloaded toolchain lives under the ordinary module cache, where Go
	// rejects overlays. Run that exact binary with a private test module cache;
	// it needs no external modules and never writes to the installed toolchain.
	targets := []string{""}
	if pkgName == "rttype" || pkgName == "reflectdata" || pkgName == "staticdata" || snapshotName == "noder_dict" {
		targets = []string{"386", "amd64", "mips", "ppc64"}
	}
	if snapshotName == "reflectdata_descriptors" {
		targets = append(targets, "amd64-split", "amd64-runtime", "amd64-dynlink")
	}
	for _, snapshotTarget = range targets {
		nativeCmd := exec.Command(filepath.Join(goroot, "bin/go"), "test", "-overlay", filepath.Join(dir, "overlay.json"), "-count=1", "-run", "^TestSyncgoNativeSnapshot$", "-v", "cmd/compile/internal/"+pkgName)
		nativeCmd.Dir = dir
		nativeCmd.Env = append(os.Environ(), "GOWORK=off", "GOTOOLCHAIN=local", "GOROOT="+goroot, "GOMODCACHE="+filepath.Join(dir, "modcache"), "SYNCGO_TARGET="+snapshotTarget)
		nativeOut, err := nativeCmd.CombinedOutput()
		if err != nil {
			t.Fatalf("native IR snapshot: %v\n%s", err, nativeOut)
		}
		want := extract(nativeOut)
		got := extract(run("test", "-race", "-count=1", "-run", "^TestSyncgoNativeSnapshot$", "-v", "./internal/"+pkgName))
		if !bytes.Equal(want, got) {
			var wantRows, gotRows []string
			if json.Unmarshal(want, &wantRows) == nil && json.Unmarshal(got, &gotRows) == nil {
				for i := 0; i < len(wantRows) && i < len(gotRows); i++ {
					if wantRows[i] != gotRows[i] {
						t.Fatalf("%s native %s %s row %d differs\nwant %s\ngot %s", s.Pin.Tag, pkgName, snapshotTarget, i, wantRows[i], gotRows[i])
					}
				}
				t.Fatalf("%s native %s %s snapshot lengths differ: want %d, got %d", s.Pin.Tag, pkgName, snapshotTarget, len(wantRows), len(gotRows))
			}
			t.Fatalf("%s native %s differs\nwant %s\ngot %s", s.Pin.Tag, pkgName, want, got)
		}
		t.Logf("%s: native %s %s snapshot matches", s.Pin.Tag, snapshotName, snapshotTarget)
	}
}

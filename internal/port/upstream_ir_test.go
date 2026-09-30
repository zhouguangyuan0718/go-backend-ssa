package port

import (
	"bytes"
	"encoding/json"
	"fmt"
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
	testUpstreamNativeGraph(t, s, r, root, "ir", "ir_symbols")
	testUpstreamNativeGraph(t, s, r, root, "noder", "noder_inline")
	testUpstreamNativeGraph(t, s, r, root, "noder", "noder_interleaved")
	testUpstreamNativeGraph(t, s, r, root, "noder", "noder_wrappers")
	testUpstreamNativeGraph(t, s, r, root, "noder", "noder_loopvar")
	testUpstreamNativeGraph(t, s, r, root, "pkginit")
	testUpstreamNativeGraph(t, s, r, root, "ssagen", "ssagen_abi")
	testUpstreamNativeGraph(t, s, r, root, "abi")
	testUpstreamNativeGraph(t, s, r, root, "deadlocals")
	testUpstreamNativeGraph(t, s, r, root, "escape")
	testUpstreamNativeGraph(t, s, r, root, "slice")
	testUpstreamNativeGraph(t, s, r, root, "compare")
	testUpstreamNativeGraph(t, s, r, root, "ssa", "ssa_value")
	testUpstreamNativeGraph(t, s, r, root, "ssa", "ssa_types")
	testUpstreamNativeGraph(t, s, r, root, "ssa", "ssa_objects")
	testUpstreamNativeGraph(t, s, r, root, "ssa", "ssa_opcodes")
	testUpstreamNativeGraph(t, s, r, root, "ssa", "ssa_graph")
	testUpstreamNativeGraph(t, s, r, root, "walk", "walk_target")
	testUpstreamNativeGraph(t, s, r, root, "walk")
	testUpstreamNativeGraph(t, s, r, root, "typecheck")
	testUpstreamNativeGraph(t, s, r, root, "pgoir")
	testUpstreamNativeGraph(t, s, r, root, "logopt")
	testUpstreamNativeGraph(t, s, r, root, "base")
	testUpstreamNativeGraph(t, s, r, root, "inline")
	testUpstreamNativeGraph(t, s, r, root, "noder")
	testUpstreamNativeGraph(t, s, r, root, "noder", "noder_dict")
	testUpstreamNativeGraph(t, s, r, root, "noder", "noder_body")
	testUpstreamNativeGraph(t, s, r, root, "noder", "noder_import")
	testUpstreamNativeGraph(t, s, r, root, "dwarfgen")
	testUpstreamNativeGraph(t, s, r, root, "objw")
	testUpstreamNativeGraph(t, s, r, root, "staticdata")
	testUpstreamNativeGraph(t, s, r, root, "staticinit", "staticinit_zero")
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
	var template []byte
	var err error
	if snapshotName == "ssa_objects" {
		template, err = ssaObjectsSnapshot(s)
	} else {
		template, err = os.ReadFile(filepath.Join(root, "internal/port/testdata/"+snapshotName+"_snapshot.go.txt"))
	}
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if snapshotName == "noder_import" || snapshotName == "noder_inline" {
		prepareNativeImport(t, root, dir, false)
	}
	template = []byte(strings.ReplaceAll(string(template), "BODY_IMPORT_PATH", fmt.Sprintf("%q", filepath.Join(dir, "reader-import"))))
	template = []byte(strings.ReplaceAll(string(template), "BODY_INPUT_PATH", fmt.Sprintf("%q", filepath.Join(dir, "reader-input"))))
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
	if pkgName == "ssa" {
		setup = `link:=&obj.Link{Arch:&obj.LinkArch{Arch:&sys.Arch{Name:target.arch,PtrSize:target.size,RegSize:target.size,ByteOrder:target.order}}};u:=types.NewUniverseWithPositions(target.size,target.size,1<<50,&link.PosTable);u.LocalPkg=u.NewPkg("example/p","p");typecheck.NewContext(ir.NewContext(u,link)).InitUniverse()`
		position = "u.Pos"
	}

	if snapshotName == "ssa_types" || snapshotName == "ssa_graph" {
		setup = strings.Replace(setup, "typecheck.NewContext(ir.NewContext(u,link)).InitUniverse()", "graph:=ir.NewContext(u,link);typecheck.NewContext(graph).InitUniverse();c:=&Context{Types:u}", 1)
	}

	if pkgName == "abi" {
		setup = `link:=&obj.Link{Arch:&obj.LinkArch{Arch:&sys.Arch{Name:target.arch,PtrSize:target.size,RegSize:target.size,ByteOrder:target.order}}};u:=types.NewUniverseWithPositions(target.size,target.size,1<<50,&link.PosTable);u.LocalPkg=u.NewPkg("example/p","p");graph:=ir.NewContext(u,link);typecheck.NewContext(graph).InitUniverse();c:=NewContext(graph)`
		position = "u.Pos"
	}
	if snapshotName == "walk_target" {
		setup = `link:=&obj.Link{Arch:&obj.LinkArch{Arch:&sys.Arch{Name:target.arch,PtrSize:target.size,RegSize:target.size,ByteOrder:target.order}}};u:=types.NewUniverseWithPositions(target.size,target.size,1<<50,&link.PosTable);u.LocalPkg=u.NewPkg("example/p","p");graph:=ir.NewContext(u,link);tc:=typecheck.NewContext(graph);tc.InitUniverse();c:=&Context{Typecheck:tc}`
		position = "u.Pos"
	}
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
	if snapshotName == "staticinit_zero" {
		setup = `link:=&obj.Link{Arch:&obj.LinkArch{Arch:&sys.Arch{Name:target.arch,PtrSize:target.size,RegSize:target.size,ByteOrder:target.order}}};u:=types.NewUniverseWithPositions(target.size,target.size,1<<50,&link.PosTable);u.LocalPkg=u.NewPkg("example/p","p");graph:=ir.NewContext(u,link);tc:=typecheck.NewContext(graph);tc.InitUniverse();rt:=rttype.NewContext(objw.NewContext(u,link,"linux"));rt.Init();c:=NewContext(tc,reflectdata.NewContext(tc,rt,staticdata.NewContext(graph,rt.Writer)))`
	}
	if pkgName == "staticdata" {
		setup = `link:=&obj.Link{Arch:&obj.LinkArch{Arch:&sys.Arch{Name:target.arch,PtrSize:target.size,RegSize:target.size,ByteOrder:target.order}}};u:=types.NewUniverseWithPositions(target.size,target.size,1<<50,&link.PosTable);u.LocalPkg=u.NewPkg("example/p","p");graph:=ir.NewContext(u,link);typecheck.NewContext(graph).InitUniverse();c:=NewContext(graph,objw.NewContext(u,link,"linux"))`
	}
	if snapshotName == "noder_dict" || snapshotName == "noder_body" || snapshotName == "noder_import" || snapshotName == "noder_inline" || snapshotName == "noder_interleaved" || snapshotName == "noder_wrappers" || snapshotName == "noder_loopvar" || snapshotName == "pkginit" || snapshotName == "ssagen_abi" || snapshotName == "deadlocals" || snapshotName == "escape" || snapshotName == "slice" || snapshotName == "compare" || snapshotName == "walk" {
		setup = `link:=&obj.Link{Arch:&obj.LinkArch{Arch:&sys.Arch{Name:target.arch,PtrSize:target.size,RegSize:target.size,ByteOrder:target.order}}};u:=types.NewUniverseWithPositions(target.size,target.size,1<<50,&link.PosTable);u.LocalPkg=u.NewPkg("example/p","p");graph:=ir.NewContext(u,link);tc:=typecheck.NewContext(graph);tc.InitUniverse();c,err:=NewContext(types2.NewContext(),tc);if err!=nil{t.Fatal(err)}`
		position = "u.Pos"
	}
	if snapshotName == "noder_body" || snapshotName == "noder_inline" || snapshotName == "noder_interleaved" || snapshotName == "noder_wrappers" || snapshotName == "noder_loopvar" || snapshotName == "pkginit" || snapshotName == "ssagen_abi" || snapshotName == "deadlocals" || snapshotName == "escape" || snapshotName == "slice" || snapshotName == "compare" || snapshotName == "walk" {
		setup += `;link.Pathname="/src";link.Pkgpath="example/p";tc.InitRuntime();c.Flag.Dwarf=true;rt:=rttype.NewContext(objw.NewContext(u,link,"linux"));rt.Init();c.ReflectData=reflectdata.NewContext(tc,rt,staticdata.NewContext(graph,rt.Writer));c.StaticInit=staticinit.NewContext(tc,c.ReflectData)`
	}

	if pkgName == "pgoir" {
		setup = `link:=&obj.Link{};u:=types.NewUniverseWithPositions(8,8,1<<50,&link.PosTable);u.LocalPkg=u.NewPkg("example/p","p");graph:=ir.NewContext(u,link);tc:=typecheck.NewContext(graph);tc.InitUniverse();c:=NewContext(tc)`
	}

	if pkgName == "logopt" {
		setup = `link:=&obj.Link{};c:=NewContext()`
	}
	if pkgName == "inline" {
		setup = `link:=&obj.Link{};u:=types.NewUniverseWithPositions(8,8,1<<50,&link.PosTable);u.LocalPkg=u.NewPkg("example/p","p");graph:=ir.NewContext(u,link);tc:=typecheck.NewContext(graph);tc.InitUniverse();c:=NewContext(tc,inlheur.NewContext(tc,pgoir.NewContext(tc)),logopt.NewContext())`
	}
	if pkgName == "base" {
		setup = `link:=&obj.Link{};c:=NewLinkContext(link)`
	}
	if snapshotName == "pkginit" || snapshotName == "ssagen_abi" || snapshotName == "deadlocals" || snapshotName == "escape" || snapshotName == "slice" || snapshotName == "compare" || snapshotName == "walk" {
		setup = strings.Replace(setup, "c,err:=NewContext(types2.NewContext(),tc)", "c,err:=noder.NewContext(types2.NewContext(),tc)", 1)
	}
	ported := strings.NewReplacer(
		"SSA_OBJ_ROOT", "go-backend-ssa/internal/cmd/obj", "OPCODE_SETUP", "c:=newOpcodeContext()",
		"IMPORT_WALK", "go-backend-ssa/internal/walk", "WALK_SETUP", "w:=walk.NewContext(c,esc)", "WALK_SET_SOFT", `w.SoftFloat=strings.HasSuffix(mode,"-soft")`, "WALK_RUN", "w.Walk(f)",
		"ZERO_BIND", "s=c.BindSchedule(s)", "ZERO_IMPORT", `"go-backend-ssa/internal/rttype";"go-backend-ssa/internal/objw";"go-backend-ssa/internal/reflectdata";"go-backend-ssa/internal/staticdata";`,
		"WALK_IMPORT", "", "WALK_BIND", "", "WALK_SOFT", "c.SoftFloat",
		"IMPORT_COMPARE", "go-backend-ssa/internal/compare", "CMP_SETUP", "cmp:=compare.NewContext(tc)", "CMP.", "cmp.",
		"IMPORT_SLICE", "go-backend-ssa/internal/slice", "SLICE_RUN", "slice.NewContext(esc,c.ReflectData).Funcs(tc.Target.Funcs)",
		"ESC_WARNINGS", `for _,d:=range c.Diagnostics.Diagnostics(){out=append(out,"warn:"+d.String())}`,
		"IMPORT_ESCAPE", "go-backend-ssa/internal/escape", "ESC_SETUP", "esc:=escape.NewContext(tc,c.Inline.Log);c.Inline.Log.Format=logopt.Json0", "ESC_RUN", "esc.Funcs(tc.Target.Funcs)", "ESC_LOG", "c.Inline.Log.Snapshot()",
		"PRE_ESCAPE", "deadlocals.NewContext(graph).Funcs(tc.Target.Funcs)",
		"IMPORT_DEADLOCALS", "go-backend-ssa/internal/deadlocals", "DEAD_RUN", "deadlocals.NewContext(graph).Funcs(fns)",
		"IMPORT_SSAGEN", "go-backend-ssa/internal/ssagen", "IMPORT_PKGINIT", "go-backend-ssa/internal/pkginit", "INIT_BEFORE_ABI", `pkginit.NewContext(c).MakeTask()`,
		"ABI_READ", `backend:=ssagen.NewContext(tc);backend.ReadFile=func(string)([]byte,error){return []byte(raw),nil};s:=backend.NewSymABIs();s.ReadSymABIs("input.symabis")`,
		"IMPORT_NODER", "go-backend-ssa/internal/noder", "PKGINIT_DRIVER_IMPORT", "",
		"PKGINIT_READ", `c.ReadLocalPackage(string(data))`,
		"PKGINIT_DRIVER", `c.Interleaved.DevirtualizeAndInlinePackage(tc.Target,nil);c.MakeWrappers(tc.Target);capture:=loopvar.NewContext(tc,c.Inline.Log);for _,fn:=range tc.Target.Funcs{capture.ForCapture(fn)}`,
		"PKGINIT_MAKE", `pi:=NewContext(c);pi.MakeTask()`, "PKGINIT_STATE.", "pi.",
		"IMPORT_LOOPVAR", "go-backend-ssa/internal/loopvar", "IMPORT_LOGOPT", "go-backend-ssa/internal/logopt",
		"LOOP_CONTEXT", `lv:=loopvar.NewContext(tc,logopt.NewContext())`, "LOOPVAR.", "lv.", "LOOP_BASE", "graph.Base",
		"LOOP_LOG_SETUP", `lv.Log.Format=logopt.Json0`,
		"LOOP_LOG_ROWS", `for _,e:=range lv.Log.Snapshot(){out=append(out,fmt.Sprintf("event:%v:%v:%s:%s:%s:%v",link.PosTable.Pos(e.Position()),link.PosTable.Pos(e.LastPosition()),e.Pass(),e.FunctionName(),e.What(),e.Arguments()))}`,
		"LOOP_WARNINGS", `for _,d:=range c.Diagnostics.Diagnostics(){out=append(out,"warn:"+d.String())}`,
		"AUTO_IMPORT", "",
		"WRAPPER_IMPORT", "",
		"DRIVER_INLINE_IMPORT", "",
		"DRIVER_IMPORT", `"bytes";"go-backend-ssa/internal/pgoir";"go-backend-ssa/internal/cmd/pgo";`,
		"DRIVER_DUMP_SETUP", `var props bytes.Buffer;graph.TraceWriter=&props;c.Debug.DumpInlFuncProps="host-stream"`,
		"DRIVER_DUMP_READ", `dump:=props.String()`,
		"DRIVER_PROFILE", `input,err:=pgo.FromSerialized(strings.NewReader(raw));if err!=nil{t.Fatal(err)};profile=c.Inline.PGO.NewProfile(input)`,
		"DRIVER.", "c.Interleaved.", "EXPAND_RUNTIME", `"go-backend-ssa/internal/rttype";"go-backend-ssa/internal/reflectdata";"go-backend-ssa/internal/objw";"go-backend-ssa/internal/staticdata";"go-backend-ssa/internal/staticinit";`, "INLINE_HEUR.", "c.Inline.Heuristics.", "EXPAND_IMPORT", "", "EXPAND_BIND", "", "INLINE.", "c.Inline.", "IMPORT_DWARF", "go-backend-ssa/internal/cmd/dwarf", "SYMBOL_IMPORT", "", "SYMBOL_TARGET", `link.TargetOS="linux"`, "PGO_HOOK_IMPORT", "", "PGO_HOOK_BIND", "", "PGO_LOOKUP", "c.Inline.PGO.LookupFunc", "PGO_CLEANUP", "c.Inline.PGO.PostLookupCleanup", "POLICY_IMPORT", `"go-backend-ssa/internal/inline/inlheur";"go-backend-ssa/internal/logopt";`, "IMPORT_PGOIR", "go-backend-ssa/internal/pgoir", "POLICY_BASE", "graph.Base", "POLICY_DEBUG", "graph.Debug", "POLICY_PGO", "c.PGO", "HASH_LINK", "link", "HEUR_IMPORT", `"go-backend-ssa/internal/inline/inlheur";`, "INLINE_ANALYZE", `c.Flag.LowerL=1;c.Inline.CanInlineFuncs(tc.Target.Funcs,nil)`, "HEUR_SETUP", `heur:=c.Inline.Heuristics`, "HEUR.", "heur.", "IMPORT_PGO", "go-backend-ssa/internal/cmd/pgo", "CG_BUILD", "c.NewProfile(input)", "AUTO_POS", "u.AutogeneratedPos", "BODY_TYPES2", `"go-backend-ssa/internal/types2";`, "BODY_RUNTIME", `"go-backend-ssa/internal/rttype";"go-backend-ssa/internal/reflectdata";"go-backend-ssa/internal/objw";"go-backend-ssa/internal/staticdata";"go-backend-ssa/internal/staticinit";`, "INIT_SCHEDULE", "c.StaticInit.NewSchedule()", "INIT_STATE", "c.StaticInit", "BODY_DRAIN", `c.ReadBodies(duringInlining,nil)`, "IMPORT_TYPES2", "go-backend-ssa/internal/types2", "DICT_CONTEXT", "c,", "DICT_ONLY", "c", "ND_DEBUG", "c.Typecheck.IR.Debug", "RD_STATIC", `"go-backend-ssa/internal/staticdata";`, "RD_FLAG", "c.Typecheck.IR.Base.Flag", "SD_FLAG", "c.IR.Base.Flag", "IMPORT_TYPEBITS", "go-backend-ssa/internal/typebits", "IMPORT_RTTYPE", "go-backend-ssa/internal/rttype", "RD_OBJW", `"go-backend-ssa/internal/objw";`, "EXPERIMENT", "u.Experiment", "TC.", "c.Typecheck.", "IMPORT_OBJW", "go-backend-ssa/internal/objw", "IMPORT_TYPECHECK", "go-backend-ssa/internal/typecheck", "RT_IR_OBJW", `"go-backend-ssa/internal/ir";"go-backend-ssa/internal/objw";`, "IMPORT_SYS", "go-backend-ssa/internal/cmd/sys", "IMPORT_OBJABI", "go-backend-ssa/internal/cmd/objabi", "IMPORT_BITVEC", "go-backend-ssa/internal/bitvec", "FIPSHASH", "link.SetFIPSDebugHash", "IMPORT_PKGBITS", "go-backend-ssa/internal/go/pkgbits", "IMPORT_BUILDCFG", "", "MAKEMARKER", "NewScopeMarker(graph.Base)", "IMPORT_IR", "go-backend-ssa/internal/ir", "GRAPH.", "graph.", "IMPORT_TYPES", "go-backend-ssa/internal/types", "IMPORT_OBJ", "go-backend-ssa/internal/cmd/obj", "IMPORT_SRC", "go-backend-ssa/internal/cmd/src", "IMPORT_BASE", "", "SETUP", setup, "UP.", "c.", "TY.", "u.", "LINK", "link", "POS", position).Replace(string(template))
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
	packages := []string{"cmd/compile/internal/ir", "cmd/compile/internal/types", "cmd/compile/internal/base", "cmd/internal/obj", "cmd/compile/internal/typecheck", "cmd/compile/internal/noder", "cmd/compile/internal/dwarfgen", "cmd/compile/internal/objw", "cmd/compile/internal/bitvec", "cmd/internal/objabi", "internal/bisect", "internal/abi", "cmd/compile/internal/rttype", "cmd/compile/internal/reflectdata", "cmd/compile/internal/typebits", "cmd/compile/internal/staticdata", "cmd/internal/hash", "cmd/compile/internal/compare", "cmd/compile/internal/staticinit", "cmd/compile/internal/pgoir", "cmd/internal/pgo", "cmd/compile/internal/inline/inlheur", "cmd/compile/internal/inline", "cmd/compile/internal/logopt", "cmd/compile/internal/devirtualize", "cmd/compile/internal/inline/interleaved", "cmd/compile/internal/loopvar", "cmd/compile/internal/pkginit", "cmd/compile/internal/ssagen", "cmd/compile/internal/deadlocals", "cmd/compile/internal/escape", "cmd/compile/internal/slice"}
	if pkgName == "ssa" {
		packages = append(packages, "cmd/compile/internal/ssa", "cmd/compile/internal/abt")
		if snapshotName == "ssa_objects" || snapshotName == "ssa_opcodes" {
			for _, arch := range []string{"arm", "arm64", "loong64", "mips", "ppc64", "riscv", "s390x", "wasm", "x86"} {
				packages = append(packages, "cmd/internal/obj/"+arch)
			}
		}
	}
	if snapshotName == "walk_target" || snapshotName == "walk" {
		packages = append(packages, "cmd/compile/internal/walk")
	}
	if pkgName == "abi" {
		packages = append(packages, "cmd/compile/internal/abi")
	}
	for _, pkg := range packages {
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
	if pkgName == "rttype" || pkgName == "reflectdata" || pkgName == "staticdata" || pkgName == "abi" || snapshotName == "staticinit_zero" || pkgName == "ssa" || snapshotName == "walk_target" {
		nativeSetup = `base.Ctxt=&obj.Link{Arch:&obj.LinkArch{Arch:&sys.Arch{Name:target.arch,PtrSize:target.size,RegSize:target.size,ByteOrder:target.order}}};types.PtrSize=target.size;types.RegSize=target.size;types.MaxWidth=1<<50;types.BuiltinPkg=types.NewPkg("go.builtin","");types.LocalPkg=types.NewPkg("example/p","p");types.UnsafePkg=types.NewPkg("unsafe","unsafe");typecheck.InitUniverse()`
	}
	if pkgName == "reflectdata" {
		nativeSetup += `;ir.Pkgs.InternalMaps=types.NewPkg("go.internal/runtime/maps","internal/runtime/maps");ir.Pkgs.InternalMaps.Prefix="internal/runtime/maps";ir.Pkgs.Runtime=types.NewPkg("go.runtime","runtime");ir.Pkgs.Runtime.Prefix="runtime";typecheck.Target=&ir.Package{};ir.Pkgs.Itab=types.NewPkg("go.itab","go.itab");ir.Pkgs.Itab.Prefix="go:itab";base.AutogeneratedPos=base.Ctxt.PosTable.XPos(src.MakePos(src.NewFileBase("<autogenerated>","<autogenerated>"),1,0));rttype.Init()`
	}
	if snapshotName == "noder_dict" || snapshotName == "noder_body" || snapshotName == "noder_import" || snapshotName == "noder_inline" || snapshotName == "noder_interleaved" || snapshotName == "noder_wrappers" || snapshotName == "noder_loopvar" || snapshotName == "pkginit" || snapshotName == "ssagen_abi" || snapshotName == "deadlocals" || snapshotName == "escape" || snapshotName == "slice" || snapshotName == "compare" || snapshotName == "walk" {
		nativeSetup = `base.Ctxt=&obj.Link{Arch:&obj.LinkArch{Arch:&sys.Arch{Name:target.arch,PtrSize:target.size,RegSize:target.size,ByteOrder:target.order}}};types.PtrSize=target.size;types.RegSize=target.size;types.MaxWidth=1<<50;types.BuiltinPkg=types.NewPkg("go.builtin","");types.LocalPkg=types.NewPkg("example/p","p");types.UnsafePkg=types.NewPkg("unsafe","unsafe");base.AutogeneratedPos=base.Ctxt.PosTable.XPos(src.MakePos(src.NewFileBase("<autogenerated>","<autogenerated>"),1,0));typecheck.InitUniverse()`
	}
	if snapshotName == "noder_import" {
		nativeSetup += `;typecheck.Target=&ir.Package{}`
	}
	if snapshotName == "noder_body" || snapshotName == "noder_inline" || snapshotName == "noder_interleaved" || snapshotName == "noder_wrappers" || snapshotName == "noder_loopvar" || snapshotName == "pkginit" || snapshotName == "ssagen_abi" || snapshotName == "deadlocals" || snapshotName == "escape" || snapshotName == "slice" || snapshotName == "compare" || snapshotName == "walk" {
		nativeSetup += `;base.Ctxt.Pathname="/src";base.Ctxt.Pkgpath="example/p";base.Flag.Dwarf=true;buildcfg.GOARCH=target.arch;buildcfg.GOOS="linux";ir.Pkgs.Go=types.NewPkg("go","");ir.Pkgs.InternalMaps=types.NewPkg("go.internal/runtime/maps","internal/runtime/maps");ir.Pkgs.InternalMaps.Prefix="internal/runtime/maps";ir.Pkgs.Runtime=types.NewPkg("go.runtime","runtime");ir.Pkgs.Runtime.Prefix="runtime";typecheck.Target=&ir.Package{};ir.Pkgs.Itab=types.NewPkg("go.itab","go.itab");ir.Pkgs.Itab.Prefix="go:itab";typecheck.InitRuntime();rttype.Init()`
	}

	if pkgName == "pgoir" {
		nativeSetup += `;typecheck.InitUniverse();typecheck.Target=&ir.Package{}`
	}

	if pkgName == "logopt" {
		nativeSetup = `base.Ctxt=&obj.Link{}`
	}
	if pkgName == "inline" {
		nativeSetup += `;typecheck.InitUniverse()`
	}
	if pkgName == "base" {
		nativeSetup = `Ctxt=&obj.Link{}`
	}
	native := strings.NewReplacer(
		"SSA_OBJ_ROOT", "cmd/internal/obj", "OPCODE_SETUP", "",
		"IMPORT_WALK", "cmd/compile/internal/walk", "WALK_SETUP", "ssagen.Arch.LinkArch=base.Ctxt.Arch", "WALK_SET_SOFT", `ssagen.Arch.SoftFloat=strings.HasSuffix(mode,"-soft")`, "WALK_RUN", "walk.Walk(f)",
		"ZERO_BIND", "", "ZERO_IMPORT", "",
		"WALK_IMPORT", `"cmd/compile/internal/ssagen";`, "WALK_BIND", "ssagen.Arch.LinkArch=base.Ctxt.Arch", "WALK_SOFT", "ssagen.Arch.SoftFloat",
		"IMPORT_COMPARE", "cmd/compile/internal/compare", "CMP_SETUP", "", "CMP.", "compare.",
		"IMPORT_SLICE", "cmd/compile/internal/slice", "SLICE_RUN", "slice.Funcs(typecheck.Target.Funcs)",
		"ESC_WARNINGS", `for _,d:=range base.SyncgoEscapeWarnings(){out=append(out,"warn:"+d)}`,
		"IMPORT_ESCAPE", "cmd/compile/internal/escape", "ESC_SETUP", `ir.EscFmt=escape.Fmt;logopt.Format=logopt.Json0`, "ESC_RUN", "escape.Funcs(typecheck.Target.Funcs)", "ESC_LOG", "logopt.SyncgoEscapeSnapshot()",
		"PRE_ESCAPE", "deadlocals.Funcs(typecheck.Target.Funcs)",
		"IMPORT_DEADLOCALS", "cmd/compile/internal/deadlocals", "DEAD_RUN", "deadlocals.Funcs(fns)",
		"IMPORT_SSAGEN", "cmd/compile/internal/ssagen", "IMPORT_PKGINIT", "cmd/compile/internal/pkginit", "INIT_BEFORE_ABI", `pkginit.MakeTask()`,
		"ABI_READ", `file:=t.TempDir()+"/input.symabis";if err:=os.WriteFile(file,[]byte(raw),0600);err!=nil{t.Fatal(err)};s:=ssagen.NewSymABIs();s.ReadSymABIs(file)`,
		"IMPORT_NODER", "cmd/compile/internal/noder", "PKGINIT_DRIVER_IMPORT", `"cmd/compile/internal/inline/interleaved";`,
		"PKGINIT_READ", `noder.SyncgoReadInit(string(data))`,
		"PKGINIT_DRIVER", `interleaved.DevirtualizeAndInlinePackage(typecheck.Target,nil);noder.MakeWrappers(typecheck.Target);for _,fn:=range typecheck.Target.Funcs{loopvar.ForCapture(fn)}`,
		"PKGINIT_MAKE", `MakeTask()`, "PKGINIT_STATE.", "",
		"IMPORT_LOOPVAR", "cmd/compile/internal/loopvar", "IMPORT_LOGOPT", "cmd/compile/internal/logopt",
		"LOOP_CONTEXT", "", "LOOPVAR.", "loopvar.", "LOOP_BASE", "base",
		"LOOP_LOG_SETUP", `logopt.Format=logopt.Json0`, "LOOP_LOG_ROWS", `out=append(out,logopt.SyncgoLoopRows(base.Ctxt)...)`,
		"LOOP_WARNINGS", `for _,d:=range base.SyncgoLoopWarnings(){out=append(out,"warn:"+d)}`,
		"AUTO_IMPORT", `"cmd/internal/src";`,
		"WRAPPER_IMPORT", `"cmd/compile/internal/inline/interleaved";`,
		"DRIVER_INLINE_IMPORT", `"cmd/compile/internal/inline";`,
		"DRIVER_IMPORT", `"path/filepath";"cmd/compile/internal/pgoir";"cmd/compile/internal/inline/interleaved";`,
		"DRIVER_DUMP_SETUP", `dumpPath:=filepath.Join(t.TempDir(),"props");base.Debug.DumpInlFuncProps=dumpPath`,
		"DRIVER_DUMP_READ", `var dump string;if data,err:=os.ReadFile(dumpPath);err==nil{dump=string(data)}else if !os.IsNotExist(err){t.Fatal(err)}`,
		"DRIVER_PROFILE", `pgoir.LookupFunc=LookupFunc;pgoir.PostLookupCleanup=PostLookupCleanup;path:=filepath.Join(t.TempDir(),"profile");if err:=os.WriteFile(path,[]byte(raw),0600);err!=nil{t.Fatal(err)};profile,err=pgoir.New(path);if err!=nil{t.Fatal(err)}`,
		"DRIVER.", "interleaved.", "EXPAND_RUNTIME", `"cmd/compile/internal/rttype";"internal/buildcfg";`, "INLINE_HEUR.", "inlheur.", "EXPAND_IMPORT", `"cmd/compile/internal/inline";"cmd/compile/internal/inline/inlheur";`, "EXPAND_BIND", `inline.InlineCall=unifiedInlineCall`, "INLINE.", "inline.", "IMPORT_DWARF", "cmd/internal/dwarf", "SYMBOL_IMPORT", `;"internal/buildcfg"`, "SYMBOL_TARGET", `buildcfg.GOOS="linux";buildcfg.GOARCH="amd64"`, "PGO_HOOK_IMPORT", `"cmd/compile/internal/pgoir";`, "PGO_HOOK_BIND", `typecheck.HaveInlineBody=unifiedHaveInlineBody;pgoir.LookupFunc=LookupFunc;pgoir.PostLookupCleanup=PostLookupCleanup`, "PGO_LOOKUP", "pgoir.LookupFunc", "PGO_CLEANUP", "pgoir.PostLookupCleanup", "POLICY_IMPORT", "", "IMPORT_PGOIR", "cmd/compile/internal/pgoir", "POLICY_BASE", "base", "POLICY_DEBUG", "base.Debug", "POLICY_PGO", "pgoir", "HASH_LINK", "Ctxt", "HEUR_IMPORT", `"cmd/compile/internal/inline/inlheur";"cmd/compile/internal/inline";`, "INLINE_ANALYZE", `base.Flag.LowerL=1;inline.CanInlineFuncs(typecheck.Target.Funcs,nil)`, "HEUR_SETUP", "typecheck.HaveInlineBody=unifiedHaveInlineBody", "HEUR.", "inlheur.", "IMPORT_PGO", "cmd/internal/pgo", "CG_BUILD", "&Profile{Profile:input,WeightedCG:createIRGraph(input.NamedEdgeMap)}", "AUTO_POS", "base.AutogeneratedPos", "BODY_TYPES2", "", "BODY_RUNTIME", `"cmd/compile/internal/rttype";"internal/buildcfg";"cmd/compile/internal/staticinit";`, "INIT_SCHEDULE", "&staticinit.Schedule{Plans:make(map[ir.Node]*staticinit.Plan),Temps:make(map[ir.Node]*ir.Name)}", "INIT_STATE", "staticinit", "BODY_DRAIN", `readBodies(typecheck.Target,duringInlining,nil)`, "IMPORT_TYPES2", "cmd/compile/internal/types2", "DICT_CONTEXT", "", "DICT_ONLY", "", "ND_DEBUG", "base.Debug", "RD_STATIC", "", "RD_FLAG", "base.Flag", "SD_FLAG", "base.Flag", "IMPORT_TYPEBITS", "cmd/compile/internal/typebits", "IMPORT_RTTYPE", "cmd/compile/internal/rttype", "RD_OBJW", "", "EXPERIMENT", "buildcfg.Experiment", "TC.", "typecheck.", "IMPORT_OBJW", "cmd/compile/internal/objw", "IMPORT_TYPECHECK", "cmd/compile/internal/typecheck", "RT_IR_OBJW", "", "IMPORT_SYS", "cmd/internal/sys", "IMPORT_OBJABI", "cmd/internal/objabi", "IMPORT_BITVEC", "cmd/compile/internal/bitvec", "FIPSHASH", "obj.SetFIPSDebugHash", "IMPORT_PKGBITS", "internal/pkgbits", "IMPORT_BUILDCFG", `;"internal/buildcfg"`, "MAKEMARKER", "ScopeMarker{}", "IMPORT_IR", "cmd/compile/internal/ir", "GRAPH.", "ir.", "IMPORT_TYPES", "cmd/compile/internal/types", "IMPORT_OBJ", "cmd/internal/obj", "IMPORT_SRC", "cmd/internal/src", "IMPORT_BASE", `"cmd/compile/internal/base"`, "SETUP", nativeSetup, "UP.", "", "TY.", "types.", "LINK", "base.Ctxt", "POS", "base.Pos").Replace(string(template))
	writeTestFile(t, dir, "reference/"+pkgName+"_snapshot_test.go", native)
	overlay[filepath.Join(goroot, "src/cmd/compile/internal/"+pkgName+"/syncgo_snapshot_test.go")] = filepath.Join(dir, "reference/"+pkgName+"_snapshot_test.go")
	// Observers expose existing buffers only in the temporary reference build.
	if snapshotName == "noder_loopvar" || snapshotName == "ssagen_abi" || snapshotName == "escape" {
		for _, observer := range []struct{ pkg, fixture string }{{"logopt", "loopvar_log_snapshot.go.txt"}, {"base", "loopvar_warning_snapshot.go.txt"}} {
			data, err := os.ReadFile(filepath.Join(root, "internal/port/testdata", observer.fixture))
			if err != nil {
				t.Fatal(err)
			}
			dest := filepath.Join("reference", observer.pkg+"_loopvar_observer.go")
			writeTestFile(t, dir, dest, string(data))
			overlay[filepath.Join(goroot, "src/cmd/compile/internal", observer.pkg, "syncgo_loopvar_observer.go")] = filepath.Join(dir, dest)
		}
	}
	if snapshotName == "pkginit" || snapshotName == "ssagen_abi" || snapshotName == "deadlocals" || snapshotName == "escape" || snapshotName == "slice" || snapshotName == "compare" || snapshotName == "walk" {
		data, err := os.ReadFile(filepath.Join(root, "internal/port/testdata/pkginit_reader.go.txt"))
		if err != nil {
			t.Fatal(err)
		}
		writeTestFile(t, dir, "reference/pkginit_reader.go", string(data))
		overlay[filepath.Join(goroot, "src/cmd/compile/internal/noder/syncgo_pkginit_reader.go")] = filepath.Join(dir, "reference/pkginit_reader.go")
	}

	if snapshotName == "escape" {
		data, err := os.ReadFile(filepath.Join(root, "internal/port/testdata/escape_log_snapshot.go.txt"))
		if err != nil {
			t.Fatal(err)
		}
		writeTestFile(t, dir, "reference/escape_log.go", string(data))
		overlay[filepath.Join(goroot, "src/cmd/compile/internal/logopt/syncgo_escape_log.go")] = filepath.Join(dir, "reference/escape_log.go")
		warning, err := os.ReadFile(filepath.Join(root, "internal/port/testdata/escape_warning_snapshot.go.txt"))
		if err != nil {
			t.Fatal(err)
		}
		writeTestFile(t, dir, "reference/escape_warning.go", string(warning))
		overlay[filepath.Join(goroot, "src/cmd/compile/internal/base/syncgo_escape_warning.go")] = filepath.Join(dir, "reference/escape_warning.go")
		printSource := "src/cmd/compile/internal/base/print.go"
		nativePrint, err := s.Read(printSource)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Count(string(nativePrint), "func FlushErrors() {") != 1 {
			t.Fatal("native diagnostic flush changed")
		}
		// Observe messages before the native flush; preserve its sorting, printing,
		// clearing, counters, and all analysis behavior.
		writeTestFile(t, dir, filepath.Join("reference", printSource), strings.Replace(string(nativePrint), "func FlushErrors() {", "func FlushErrors() {\nsyncgoCaptureEscapeWarnings()", 1))

	}
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
	if pkgName == "rttype" || pkgName == "reflectdata" || pkgName == "staticdata" || pkgName == "abi" || snapshotName == "staticinit_zero" || pkgName == "ssa" || snapshotName == "walk_target" || snapshotName == "noder_dict" || snapshotName == "noder_body" || snapshotName == "noder_inline" || snapshotName == "noder_interleaved" || snapshotName == "noder_wrappers" || snapshotName == "noder_loopvar" || snapshotName == "pkginit" || snapshotName == "ssagen_abi" || snapshotName == "deadlocals" || snapshotName == "escape" || snapshotName == "slice" || snapshotName == "compare" || snapshotName == "walk" {
		targets = []string{"386", "amd64", "mips", "ppc64"}
	}
	if snapshotName == "ssa_objects" || snapshotName == "ssa_opcodes" {
		targets = []string{""} // Every architecture is checked in one constant snapshot.
	}
	if snapshotName == "reflectdata_descriptors" {
		targets = append(targets, "amd64-split", "amd64-runtime", "amd64-dynlink")
	}
	if snapshotName == "noder_body" {
		targets = append(targets, "amd64-nosync", "amd64-fips", "amd64-nooutline", "amd64-stressoutline", "amd64-duringinline", "amd64-sharedinline", "amd64-newinline")
		fixture, err := os.ReadFile(filepath.Join(root, "internal/port/testdata/noder_body_fixture.go.txt"))
		if err != nil {
			t.Fatal(err)
		}
		writeTestFile(t, dir, "internal/noder/adapter_fixture_test.go", strings.ReplaceAll(string(fixture), "BODY_INPUT_PATH", fmt.Sprintf("%q", filepath.Join(dir, "reader-input"))))
	}
	if snapshotName == "noder_inline" {
		targets = append(targets, "amd64-newinline")
		fixture, err := os.ReadFile(filepath.Join(root, "internal/port/testdata/noder_inline_fixture.go.txt"))
		if err != nil {
			t.Fatal(err)
		}
		writeTestFile(t, dir, "internal/noder/adapter_fixture_test.go", strings.ReplaceAll(string(fixture), "BODY_INPUT_PATH", fmt.Sprintf("%q", filepath.Join(dir, "reader-input"))))
	}
	if snapshotName == "noder_interleaved" {
		targets = append(targets, "amd64-newinline", "amd64-noinline", "amd64-single", "amd64-pgo")
		fixture, err := os.ReadFile(filepath.Join(root, "internal/port/testdata/noder_interleaved_fixture.go.txt"))
		if err != nil {
			t.Fatal(err)
		}
		writeTestFile(t, dir, "internal/noder/adapter_fixture_test.go", strings.ReplaceAll(string(fixture), "BODY_INPUT_PATH", fmt.Sprintf("%q", filepath.Join(dir, "reader-input"))))
	}

	if snapshotName == "noder_wrappers" {
		targets = append(targets, "amd64-newinline", "amd64-noinline", "ppc64-dynlink", "wasm", "amd64-runtime", "amd64-import")
		fixture, err := os.ReadFile(filepath.Join(root, "internal/port/testdata/noder_wrappers_fixture.go.txt"))
		if err != nil {
			t.Fatal(err)
		}
		writeTestFile(t, dir, "internal/noder/adapter_fixture_test.go", strings.ReplaceAll(string(fixture), "BODY_INPUT_PATH", fmt.Sprintf("%q", filepath.Join(dir, "reader-input"))))
	}
	if snapshotName == "noder_loopvar" {
		targets = append(targets, "amd64-newinline", "amd64-hash0", "amd64-hash1", "amd64-exclude", "amd64-force")
		fixture, err := os.ReadFile(filepath.Join(root, "internal/port/testdata/noder_loopvar_fixture.go.txt"))
		if err != nil {
			t.Fatal(err)
		}
		writeTestFile(t, dir, "internal/noder/adapter_fixture_test.go", strings.ReplaceAll(string(fixture), "BODY_INPUT_PATH", fmt.Sprintf("%q", filepath.Join(dir, "reader-input"))))
	}
	if snapshotName == "pkginit" {
		targets = append(targets, "amd64-asan", "386-asan", "amd64-empty", "amd64-main", "amd64-runtime", "amd64-deps", "amd64-deps-empty", "amd64-fuzz", "amd64-asan-empty")
		fixture, err := os.ReadFile(filepath.Join(root, "internal/port/testdata/pkginit_fixture.go.txt"))
		if err != nil {
			t.Fatal(err)
		}
		writeTestFile(t, dir, "internal/noder/adapter_fixture_test.go", strings.ReplaceAll(string(fixture), "BODY_INPUT_PATH", fmt.Sprintf("%q", filepath.Join(dir, "reader-input"))))
	}
	if snapshotName == "ssagen_abi" {
		targets = append(targets, "amd64-noregabi", "ppc64-dynlink", "amd64-cgo", "amd64-linkname", "amd64-conflict", "amd64-duplicates")
		fixture, err := os.ReadFile(filepath.Join(root, "internal/port/testdata/ssagen_abi_fixture.go.txt"))
		if err != nil {
			t.Fatal(err)
		}
		writeTestFile(t, dir, "internal/noder/adapter_fixture_test.go", strings.ReplaceAll(string(fixture), "BODY_INPUT_PATH", fmt.Sprintf("%q", filepath.Join(dir, "reader-input"))))
	}
	if snapshotName == "deadlocals" {
		targets = append(targets, "amd64-noopt", "amd64-disabled", "amd64-closureonly")
		fixture, err := os.ReadFile(filepath.Join(root, "internal/port/testdata/deadlocals_fixture.go.txt"))
		if err != nil {
			t.Fatal(err)
		}
		writeTestFile(t, dir, "internal/noder/adapter_fixture_test.go", strings.ReplaceAll(string(fixture), "BODY_INPUT_PATH", fmt.Sprintf("%q", filepath.Join(dir, "reader-input"))))
	}
	if snapshotName == "escape" {
		targets = append(targets, "amd64-nocopy", "amd64-diagnostics", "amd64-runtime", "amd64-freegc", "amd64-inline", "amd64-inline-runtime")
		fixture, err := os.ReadFile(filepath.Join(root, "internal/port/testdata/escape_fixture.go.txt"))
		if err != nil {
			t.Fatal(err)
		}
		writeTestFile(t, dir, "internal/noder/adapter_fixture_test.go", strings.ReplaceAll(string(fixture), "BODY_INPUT_PATH", fmt.Sprintf("%q", filepath.Join(dir, "reader-input"))))
	}
	if snapshotName == "slice" {
		targets = append(targets, "amd64-noopt", "amd64-small", "amd64-exclude", "amd64-all", "amd64-inline")
		fixture, err := os.ReadFile(filepath.Join(root, "internal/port/testdata/slice_fixture.go.txt"))
		if err != nil {
			t.Fatal(err)
		}
		writeTestFile(t, dir, "internal/noder/adapter_fixture_test.go", strings.ReplaceAll(string(fixture), "BODY_INPUT_PATH", fmt.Sprintf("%q", filepath.Join(dir, "reader-input"))))
	}
	if snapshotName == "walk" {
		targets = append(targets, "amd64-soft", "amd64-noopt", "amd64-inline", "amd64-fips", "amd64-jumptable", "amd64-split", "amd64-libfuzzer")
		fixture, err := os.ReadFile(filepath.Join(root, "internal/port/testdata/walk_fixture.go.txt"))
		if err != nil {
			t.Fatal(err)
		}
		writeTestFile(t, dir, "internal/noder/adapter_fixture_test.go", strings.ReplaceAll(string(fixture), "BODY_INPUT_PATH", fmt.Sprintf("%q", filepath.Join(dir, "reader-input"))))
	}
	if snapshotName == "compare" {
		targets = append(targets, "amd64-aligned", "amd64-nomerge", "amd64-large")
		fixture, err := os.ReadFile(filepath.Join(root, "internal/port/testdata/compare_fixture.go.txt"))
		if err != nil {
			t.Fatal(err)
		}
		writeTestFile(t, dir, "internal/noder/adapter_fixture_test.go", strings.ReplaceAll(string(fixture), "BODY_INPUT_PATH", fmt.Sprintf("%q", filepath.Join(dir, "reader-input"))))
	}
	for _, snapshotTarget = range targets {
		if snapshotName == "escape" {
			// The library selects this build-time feature per instance. Only force
			// the upstream experiment gate for this reference configuration; retain
			// the complete pinned algorithm and all other native build constants.
			source := "src/cmd/compile/internal/escape/escape.go"
			data, err := s.Read(source)
			if err != nil {
				t.Fatal(err)
			}
			code := string(data)
			if strings.HasSuffix(snapshotTarget, "-freegc") {
				if strings.Count(code, "if goexperiment.RuntimeFreegc {") != 1 {
					t.Fatal("runtimefreegc gate changed")
				}
				code = strings.Replace(code, "if goexperiment.RuntimeFreegc {", "if true {", 1)
				code = strings.Replace(code, `"internal/goexperiment"`, `_ "internal/goexperiment"`, 1)
			}
			writeTestFile(t, dir, filepath.Join("reference", source), code)
		}

		if snapshotName == "noder_wrappers" && strings.HasSuffix(snapshotTarget, "-import") {
			prepareNativeImport(t, root, dir, false, "noder_wrappers_import.go.txt")
		}
		if snapshotName == "noder_inline" && strings.HasSuffix(snapshotTarget, "-newinline") {
			prepareNativeImport(t, root, dir, true)
		}
		if snapshotName == "noder_body" || snapshotName == "noder_inline" || snapshotName == "noder_interleaved" || snapshotName == "noder_wrappers" || snapshotName == "noder_loopvar" || snapshotName == "pkginit" || snapshotName == "ssagen_abi" || snapshotName == "deadlocals" || snapshotName == "escape" || snapshotName == "slice" || snapshotName == "compare" || snapshotName == "walk" {
			run("test", "-count=1", "-run", "^TestSyncgoBodyFixture$", "./internal/noder")
		}

		nativeCmd := exec.Command(filepath.Join(goroot, "bin/go"), "test", "-overlay", filepath.Join(dir, "overlay.json"), "-count=1", "-run", "^TestSyncgoNativeSnapshot$", "-v", "cmd/compile/internal/"+pkgName)
		nativeCmd.Dir = dir
		nativeCmd.Env = append(os.Environ(), "GOWORK=off", "GOTOOLCHAIN=local", "GOROOT="+goroot, "GOMODCACHE="+filepath.Join(dir, "modcache"), "SYNCGO_TARGET="+snapshotTarget)
		nativeOut, err := nativeCmd.CombinedOutput()
		if snapshotName == "compare" && strings.HasSuffix(snapshotTarget, "-large") {
			const message = "SubstArgTypes: too many argument types"
			if err == nil || !bytes.Contains(nativeOut, []byte(message)) {
				t.Fatalf("expected pinned comparison failure, got %v: %s", err, nativeOut)
			}
			portCmd := exec.Command("go", "test", "-race", "-count=1", "-run", "^TestSyncgoNativeSnapshot$", "-v", "./internal/compare")
			portCmd.Dir = dir
			portCmd.Env = append(os.Environ(), "GOWORK=off", "GOTOOLCHAIN=go1.27.1", "SYNCGO_TARGET="+snapshotTarget)
			portOut, portErr := portCmd.CombinedOutput()
			if portErr == nil || !bytes.Contains(portOut, []byte(message)) {
				t.Fatalf("migrated comparison failure differs: %v: %s", portErr, portOut)
			}
			t.Logf("%s: native compare %s failure matches", s.Pin.Tag, snapshotTarget)
			continue
		}
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

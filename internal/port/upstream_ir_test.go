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
}
func testUpstreamNativeGraph(t *testing.T, s *Source, r *Rendered, root, pkgName string) {
	t.Helper()
	template, err := os.ReadFile(filepath.Join(root, "internal/port/testdata/"+pkgName+"_snapshot.go.txt"))
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
	ported := strings.NewReplacer("IMPORT_IR", "go-backend-ssa/internal/ir", "GRAPH.", "graph.", "IMPORT_TYPES", "go-backend-ssa/internal/types", "IMPORT_OBJ", "go-backend-ssa/internal/cmd/obj", "IMPORT_SRC", "go-backend-ssa/internal/cmd/src", "IMPORT_BASE", "", "SETUP", setup, "UP.", "c.", "TY.", "u.", "LINK", "link", "POS", position).Replace(string(template))
	writeTestFile(t, dir, "internal/"+pkgName+"/adapter_snapshot_test.go", ported)
	run := func(args ...string) []byte {
		cmd := exec.Command("go", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GOWORK=off", "GOTOOLCHAIN=go1.27.1")
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("%v: %v\n%s", args, err, out)
		}
		return out
	}
	goroot := strings.TrimSpace(string(run("env", "GOROOT")))
	overlay := map[string]string{}
	for _, pkg := range []string{"cmd/compile/internal/ir", "cmd/compile/internal/types", "cmd/compile/internal/base", "cmd/internal/obj", "cmd/compile/internal/typecheck"} {
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
	native := strings.NewReplacer("IMPORT_IR", "cmd/compile/internal/ir", "GRAPH.", "ir.", "IMPORT_TYPES", "cmd/compile/internal/types", "IMPORT_OBJ", "cmd/internal/obj", "IMPORT_SRC", "cmd/internal/src", "IMPORT_BASE", `"cmd/compile/internal/base"`, "SETUP", nativeSetup, "UP.", "", "TY.", "types.", "LINK", "base.Ctxt", "POS", "base.Pos").Replace(string(template))
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
	nativeCmd := exec.Command(filepath.Join(goroot, "bin/go"), "test", "-overlay", filepath.Join(dir, "overlay.json"), "-count=1", "-run", "^TestSyncgoNativeSnapshot$", "-v", "cmd/compile/internal/"+pkgName)
	nativeCmd.Dir = dir
	nativeCmd.Env = append(os.Environ(), "GOWORK=off", "GOTOOLCHAIN=local", "GOROOT="+goroot, "GOMODCACHE="+filepath.Join(dir, "modcache"))
	nativeOut, err := nativeCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("native IR snapshot: %v\n%s", err, nativeOut)
	}
	want := extract(nativeOut)
	got := extract(run("test", "-race", "-count=1", "-run", "^TestSyncgoNativeSnapshot$", "-v", "./internal/"+pkgName))
	if !bytes.Equal(want, got) {
		t.Fatalf("%s native %s differs\nwant %s\ngot %s", s.Pin.Tag, pkgName, want, got)
	}
	t.Logf("%s: native %s snapshot matches", s.Pin.Tag, pkgName)
}

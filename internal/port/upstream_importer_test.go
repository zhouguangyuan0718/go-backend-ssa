package port

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestPinnedTypeImporter(t *testing.T) {
	repo := os.Getenv("GO_SSA_SOURCE_REPO")
	if repo == "" {
		t.Skip("set GO_SSA_SOURCE_REPO for native type importer differential")
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
			testUpstreamTypeImporter(t, s, r, root)
		})
	}
}
func testUpstreamTypeImporter(t *testing.T, s *Source, r *Rendered, root string) {
	t.Helper()
	dir := t.TempDir()
	for name, b := range r.Files {
		writeTestFile(t, dir, name, string(b))
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
	writeTestFile(t, dir, "go.mod", "module go-backend-ssa\n\ngo 1.27.1\n")
	run := func(args ...string) []byte {
		t.Helper()
		cmd := exec.Command("go", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GOWORK=off", "GOTOOLCHAIN=go1.27.1", "IMPORTER_EXPORT_DIR="+dir)
		b, e := cmd.CombinedOutput()
		if e != nil {
			t.Fatalf("%v: %v\n%s", args, e, b)
		}
		return b
	}
	for _, name := range []string{"a", "b"} {
		raw, e := os.ReadFile(filepath.Join(root, "internal/importer/testdata", name+".go.txt"))
		if e != nil {
			t.Fatal(e)
		}
		writeTestFile(t, dir, name+".go", string(raw))
		args := []string{"tool", "compile", "-p", "example/" + name, "-trimpath", dir + "=>/src", "-pack", "-o", filepath.Join(dir, name+".a")}
		if name == "b" {
			writeTestFile(t, dir, "importcfg", "packagefile example/a="+filepath.Join(dir, "a.a")+"\n")
			args = append(args, "-importcfg", filepath.Join(dir, "importcfg"))
		}
		run(append(args, filepath.Join(dir, name+".go"))...)
		archive, e := os.ReadFile(filepath.Join(dir, name+".a"))
		if e != nil {
			t.Fatal(e)
		}
		start, end := bytes.Index(archive, []byte("\n$$B\nu")), bytes.Index(archive, []byte("\n$$\n"))
		if start < 0 || end <= start {
			t.Fatal("invalid compiler archive")
		}
		writeTestFile(t, dir, name+".u", string(archive[start+len("\n$$B\nu"):end]))
	}
	template, e := os.ReadFile("testdata/importer_snapshot.go.txt")
	if e != nil {
		t.Fatal(e)
	}
	ported := strings.NewReplacer("IMPORT_TYPES2", "go-backend-ssa/internal/types2", "IMPORT_SYNTAX", "go-backend-ssa/internal/syntax", "IMPORT_PKGBITS", "go-backend-ssa/internal/go/pkgbits", "SETUP_READER", "c:=NewContext(checker)", "READ_PACKAGE", "c.ReadPackage", "INSTANTIATE", "checker.Instantiate", "BASIC_INT", "checker.Typ[types2.Int]", "IDENTICAL", "checker.Identical", "SIZES_FOR", "checker.SizesFor", "UNSAFE_PACKAGE", "checker.Unsafe").Replace(string(template))
	native := strings.NewReplacer("IMPORT_TYPES2", "cmd/compile/internal/types2", "IMPORT_SYNTAX", "cmd/compile/internal/syntax", "IMPORT_PKGBITS", "internal/pkgbits", "SETUP_READER", "", "READ_PACKAGE", "ReadPackage", "INSTANTIATE", "types2.Instantiate", "BASIC_INT", "types2.Typ[types2.Int]", "IDENTICAL", "types2.Identical", "SIZES_FOR", "types2.SizesFor", "UNSAFE_PACKAGE", "types2.Unsafe").Replace(string(template))
	native = strings.Replace(native, `"testing"`, `"testing";"cmd/compile/internal/base"`, 1)
	native = strings.Replace(native, `var out []string`, `base.Debug.Panic=1;var out []string`, 1)
	writeTestFile(t, dir, "internal/importer/adapter_snapshot_test.go", ported)
	goroot := strings.TrimSpace(string(run("env", "GOROOT")))
	overlay := map[string]string{}
	for _, pkg := range []string{"cmd/compile/internal/importer", "cmd/compile/internal/types2", "cmd/compile/internal/syntax", "internal/pkgbits", "internal/types/errors", "internal/goversion"} {
		for _, name := range s.PackageFiles(pkg) {
			b, e := s.Read(name)
			if e != nil {
				t.Fatal(e)
			}
			dest := filepath.Join("reference", name)
			writeTestFile(t, dir, dest, string(b))
			overlay[filepath.Join(goroot, name)] = filepath.Join(dir, dest)
		}
	}
	writeTestFile(t, dir, "reference/importer_snapshot_test.go", native)
	overlay[filepath.Join(goroot, "src/cmd/compile/internal/importer/syncgo_snapshot_test.go")] = filepath.Join(dir, "reference/importer_snapshot_test.go")
	b, e := json.Marshal(struct{ Replace map[string]string }{overlay})
	if e != nil {
		t.Fatal(e)
	}
	writeTestFile(t, dir, "overlay.json", string(b))
	cmd := exec.Command(filepath.Join(goroot, "bin/go"), "test", "-overlay", filepath.Join(dir, "overlay.json"), "-count=1", "-run", "^TestSyncgoImporterSnapshot$", "-v", "cmd/compile/internal/importer")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOWORK=off", "GOTOOLCHAIN=local", "GOROOT="+goroot, "GOMODCACHE="+filepath.Join(dir, "modcache"), "IMPORTER_EXPORT_DIR="+dir)
	want, e := cmd.CombinedOutput()
	if e != nil {
		t.Fatalf("native importer: %v\n%s", e, want)
	}
	got := run("test", "-race", "-count=1", "-run", "^TestSyncgoImporterSnapshot$", "-v", "./internal/importer")
	extract := func(out []byte) []byte {
		for _, line := range bytes.Split(out, []byte{'\n'}) {
			if b, ok := bytes.CutPrefix(line, []byte("SYNCGO_IMPORTER=")); ok {
				return b
			}
		}
		t.Fatalf("no snapshot: %s", out)
		return nil
	}
	if !bytes.Equal(extract(want), extract(got)) {
		t.Fatalf("type import differs\nnative %s\nported %s", want, got)
	}
	var rows []string
	if e = json.Unmarshal(extract(got), &rows); e != nil {
		t.Fatal(e)
	}
	t.Log(fmt.Sprintf("%s native lazy-import differential: 8 order/layout combinations, %d observations match", s.Pin.Tag, len(rows)))
}

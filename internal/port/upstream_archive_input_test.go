package port

import (
	"context"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestPinnedArchiveInput(t *testing.T) {
	repo := os.Getenv("GO_SSA_SOURCE_REPO")
	if repo == "" {
		t.Skip("set GO_SSA_SOURCE_REPO for native archive input differential")
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
			testUpstreamArchiveInput(t, s, r, root)
		})
	}
}
func testUpstreamArchiveInput(t *testing.T, s *Source, r *Rendered, root string) {
	t.Helper()
	dir := t.TempDir()
	for name, b := range r.Files {
		if strings.HasPrefix(name, "internal/cmd/bio/") || strings.HasPrefix(name, "internal/go/exportdata/") || strings.HasPrefix(name, "internal/go/saferio/") {
			writeTestFile(t, dir, name, string(b))
		}
	}
	b, e := os.ReadFile(filepath.Join(root, "internal/cmd/bio/adapter_context.go"))
	if e != nil {
		t.Fatal(e)
	}
	writeTestFile(t, dir, "internal/cmd/bio/adapter_context.go", string(b))
	units := []Unit{
		{Source: "src/cmd/internal/bio/buf.go", Functions: []string{"NewReader"}, Declarations: []string{"Reader", "Reader.Offset", "Reader.MustSeek", "Reader.Close", "Reader.File"}},
		{Source: "src/internal/exportdata/exportdata.go", Functions: []string{"ReadUnified", "FindPackageDefinition", "ReadObjectHeaders", "ReadExportDataHeader"}},
		{Source: "src/internal/exportdata/support.go", Functions: []string{"readArchiveHeader"}},
		{Source: "src/internal/saferio/io.go", Functions: []string{"ReadData", "ReadDataAt"}, Declarations: []string{"chunk"}},
	}
	for _, u := range units {
		b, e := s.Read(u.Source)
		if e != nil {
			t.Fatal(e)
		}
		fs := token.NewFileSet()
		f, e := selectFile(fs, u.Source, b, u)
		if e != nil {
			t.Fatal(e)
		}
		pruneImports(f)
		code := nodeText(fs, f)
		code = strings.ReplaceAll(code, `"internal/saferio"`, `"go-backend-ssa/internal/reference/saferio"`)
		pkg := filepath.Base(filepath.Dir(u.Source))
		writeTestFile(t, dir, "internal/reference/"+pkg+"/"+filepath.Base(u.Source), code)
	}
	fixture, e := os.ReadFile("testdata/archive_input_test.go.txt")
	if e != nil {
		t.Fatal(e)
	}
	writeTestFile(t, dir, "archiveinput/input_test.go", string(fixture))
	writeTestFile(t, dir, "go.mod", "module go-backend-ssa\n\ngo 1.27.1\n")
	cmd := exec.Command("go", "test", "-race", "-v", "./archiveinput")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOWORK=off", "GOTOOLCHAIN=go1.27.1")
	if b, e := cmd.CombinedOutput(); e != nil {
		t.Fatalf("%v\n%s", e, b)
	} else {
		t.Logf("%s", b)
	}
}

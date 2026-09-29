package port

import (
	"go/ast"
	"go/token"
	"os"
	"os/exec"
	"testing"
)

// Run upstream's own position regression cases against the installed algorithms.
// Only the explicitly removed zero sentinels in the test expressions are adapted;
// test functions, expected values and assertions are kept from the pinned source.
func testUpstreamPositions(t *testing.T, s *Source, r *Rendered) {
	t.Helper()
	units := []Unit{
		{Source: "src/cmd/internal/src/pos.go", WholeFile: true, ZeroValues: []string{"NoPos", "noPos"}},
		{Source: "src/cmd/internal/src/xpos.go", WholeFile: true, ZeroValues: []string{"NoXPos"}},
		{Source: "src/cmd/internal/src/pos_test.go", WholeFile: true},
		{Source: "src/cmd/internal/src/xpos_test.go", WholeFile: true},
	}
	fs := token.NewFileSet()
	var files []*ast.File
	for _, u := range units {
		data, err := s.Read(u.Source)
		if err != nil {
			t.Fatal(err)
		}
		f, err := selectFile(fs, u.Source, data, u)
		if err != nil {
			t.Fatal(err)
		}
		files = append(files, f)
	}
	_, info, err := checkTypes(fs, "cmd/internal/src", files, newImporter())
	if err != nil {
		t.Fatal(err)
	}
	if err := replaceZeroValues(fs, files, units, info); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	writeTestFile(t, dir, "go.mod", "module go-backend-ssa\n\ngo 1.27.1\n")
	for i, u := range units {
		data := r.Files[u.output()]
		if i >= 2 {
			data, err = transform(fs, files[i], info, u, nil)
			if err != nil {
				t.Fatal(err)
			}
		}
		writeTestFile(t, dir, u.output(), string(data))
	}
	cmd := exec.Command("go", "test", "-race", "-v", "./internal/cmd/src")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOWORK=off", "GOTOOLCHAIN=go1.27.1")
	b, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("upstream position suite (%s): %v\n%s", s.Pin.Tag, err, b)
	}
	t.Logf("upstream position suite at %s:\n%s", s.Pin.Tag, b)
}

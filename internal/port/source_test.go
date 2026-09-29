package port

import (
	"context"
	"os/exec"
	"strings"
	"testing"
)

func fixtureRepo(t *testing.T, files map[string]string) (string, Pin) {
	t.Helper()
	dir := t.TempDir()
	for name, data := range files {
		writeTestFile(t, dir, name, data)
	}
	writeTestFile(t, dir, "VERSION", "go1.27.1\n")
	for _, args := range [][]string{{"init", "-q"}, {"add", "."}, {"-c", "user.name=Test", "-c", "user.email=test@example.invalid", "-c", "commit.gpgsign=false", "commit", "-qm", "fixture"}, {"tag", "go1.27.1"}} {
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		if b, e := cmd.CombinedOutput(); e != nil {
			t.Fatalf("git: %v %s", e, b)
		}
	}
	commit, err := git(context.Background(), dir, nil, "rev-parse", "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	return dir, Pin{Tag: "go1.27.1", Commit: strings.TrimSpace(string(commit)), BuildToolchain: "go1.27.1", TargetModule: "go-backend-ssa"}
}

func TestPinnedObjectsIgnoreWorkingTree(t *testing.T) {
	repo, pin := fixtureRepo(t, map[string]string{"src/p/p.go": "package p\n"})
	writeTestFile(t, repo, "src/p/p.go", "dirty source, not Go")
	before, _ := git(context.Background(), repo, nil, "status", "--porcelain=v1")
	s, err := OpenSource(context.Background(), repo, pin)
	if err != nil {
		t.Fatal(err)
	}
	b, err := s.Read("src/p/p.go")
	if err != nil || string(b) != "package p\n" {
		t.Fatalf("read dirty tree: %q %v", b, err)
	}
	after, _ := git(context.Background(), repo, nil, "status", "--porcelain=v1")
	if string(before) != string(after) {
		t.Fatal("changed source worktree")
	}
	pin.Commit = strings.Repeat("0", 40)
	if _, err = OpenSource(context.Background(), repo, pin); err == nil {
		t.Fatal("accepted wrong commit")
	}
	pin.Tag = "go1.26.5"
	if _, err = OpenSource(context.Background(), repo, pin); err == nil {
		t.Fatal("accepted unsupported language baseline")
	}
	if _, err = s.Read("../VERSION"); err == nil {
		t.Fatal("accepted source traversal")
	}
}

func TestSourceCancellation(t *testing.T) {
	repo, pin := fixtureRepo(t, nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := OpenSource(ctx, repo, pin); err == nil {
		t.Fatal("ignored cancellation")
	}
}

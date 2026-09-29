package port

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Opt-in, offline: GO_SSA_SOURCE_REPO points at a local repository containing
// both official tags. Ordinary unit tests need no checkout or network.
func TestPinnedUpstreamAndUpgrade(t *testing.T) {
	repo := os.Getenv("GO_SSA_SOURCE_REPO")
	if repo == "" {
		t.Skip("set GO_SSA_SOURCE_REPO for real pinned-source/upgrade tests")
	}
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	pin, plan := Baseline(), Migration()
	ctx := context.Background()
	before, err := git(ctx, repo, nil, "status", "--porcelain=v1")
	if err != nil {
		t.Fatal(err)
	}
	s, err := OpenSource(ctx, repo, pin)
	if err != nil {
		t.Fatal(err)
	}
	r, err := Render(s, plan, root)
	if err != nil {
		t.Fatal(err)
	}
	if err := Check(root, r); err != nil {
		t.Fatal(err)
	}
	r2, err := Render(s, plan, root)
	if err != nil {
		t.Fatal(err)
	}
	for p, b := range r.Files {
		if string(b) != string(r2.Files[p]) {
			t.Fatalf("non-reproducible render: %s", p)
		}
	}
	testUpstreamPositions(t, s, r)
	testUpstreamSymbols(t, s, r, root)
	testUpstreamSSAState(t, s, r, root)
	testUpstreamSyntax(t, s, r)
	testUpstreamTypes2Tables(t, s)
	testUpstreamChecker(t, s, r, root)
	oldPin := pin
	oldPin.Tag = "go1.27.0"
	commit, err := git(ctx, repo, nil, "rev-parse", "refs/tags/go1.27.0^{commit}")
	if err != nil {
		t.Fatal(err)
	}
	oldPin.Commit = strings.TrimSpace(string(commit))
	oldSource, err := OpenSource(ctx, repo, oldPin)
	if err != nil {
		t.Fatal(err)
	}
	old, err := Render(oldSource, plan, root)
	if err != nil {
		t.Fatalf("unchanged rules on go1.27.0: %v", err)
	}
	testUpstreamPositions(t, oldSource, old)
	testUpstreamSymbols(t, oldSource, old, root)
	testUpstreamSSAState(t, oldSource, old, root)
	testUpstreamSyntax(t, oldSource, old)
	testUpstreamTypes2Tables(t, oldSource)
	testUpstreamChecker(t, oldSource, old, root)
	dir := t.TempDir()
	if err := Install(dir, old); err != nil {
		t.Fatal(err)
	}
	if err := Check(dir, old); err != nil {
		t.Fatal(err)
	}
	if err := Install(dir, r); err != nil {
		t.Fatal(err)
	}
	if err := Check(dir, r); err != nil {
		t.Fatal(err)
	}
	changed := 0
	for name, b := range r.Files {
		if strings.HasSuffix(name, ".go") && string(b) != string(old.Files[name]) {
			changed++
		}
	}
	after, err := git(ctx, repo, nil, "status", "--porcelain=v1")
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatal("source worktree changed")
	}
	t.Logf("go1.27.0 -> go1.27.1: %d output Go files changed; same selection/context rules; source tree preserved", changed)
}

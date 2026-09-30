package port

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPinnedMidway(t *testing.T) {
	repo := os.Getenv("GO_SSA_SOURCE_REPO")
	if repo == "" {
		t.Skip("set GO_SSA_SOURCE_REPO for native midway differential")
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
			testUpstreamNativeGraph(t, s, r, root, "midway")
		})
	}
}

package port

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestPinnedSSAConstruction offers a focused offline check of the native construction boundary and resumed machine-code emission.
// The complete upgrade suite also executes these snapshots.
func TestPinnedSSAConstruction(t *testing.T) {
	repo := os.Getenv("GO_SSA_SOURCE_REPO")
	if repo == "" {
		t.Skip("set GO_SSA_SOURCE_REPO for pinned SSA construction differential")
	}
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	for _, tag := range []string{"go1.27.1", "go1.27.0"} {
		t.Run(tag, func(t *testing.T) {
			pin := Baseline()
			if tag != pin.Tag {
				pin.Tag = tag
				commit, err := git(context.Background(), repo, nil, "rev-parse", "refs/tags/"+tag+"^{commit}")
				if err != nil {
					t.Fatal(err)
				}
				pin.Commit = strings.TrimSpace(string(commit))
			}
			s, err := OpenSource(context.Background(), repo, pin)
			if err != nil {
				t.Fatal(err)
			}
			r, err := Render(s, Migration(), root)
			if err != nil {
				t.Fatal(err)
			}
			t.Run("built", func(t *testing.T) { testUpstreamNativeGraph(t, s, r, root, "ssagen", "ssagen_construction") })
			t.Run("resumed", func(t *testing.T) { testUpstreamNativeGraph(t, s, r, root, "ssagen", "ssagen_resume") })
			t.Run("early", func(t *testing.T) { testUpstreamNativeGraph(t, s, r, root, "ssagen", "ssagen_early") })
			t.Run("early-resumed", func(t *testing.T) { testUpstreamNativeGraph(t, s, r, root, "ssagen", "ssagen_early_resume") })
		})
	}
}

// Every observer build, including native intrinsic bodies, uses the held graph.
func constructionObserver(t *testing.T, code string, resume, early bool) string {
	t.Helper()
	for _, hot := range []string{"hot", "false"} {
		old := "c.buildssa(fn,0," + hot + ")"
		if !strings.Contains(code, old) {
			t.Fatal("observer build boundary changed", old)
		}
		suffix := ".Func()"
		if resume {
			suffix = ".ContinueNative()"
		}
		replacement := "c.ConstructSSA(fn," + hot + ")" + suffix
		if early {
			if resume {
				replacement = "func()*ssa.Func{h:=c.ConstructSSA(fn," + hot + ");h.EarlyOptimize();return h.ContinueNative()}()"
			} else {
				replacement = "c.ConstructSSA(fn," + hot + ").EarlyOptimize()"
			}
		}
		code = strings.ReplaceAll(code, old, replacement)
	}
	return code
}

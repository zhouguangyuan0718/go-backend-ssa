package base

import (
	"bytes"
	"go-backend-ssa/internal/cmd/obj"
	"go-backend-ssa/internal/cmd/src"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestHashDebugIndependentContexts(t *testing.T) {
	var wg sync.WaitGroup
	for _, file := range []string{"first.go", "second.go"} {
		wg.Go(func() {
			c := NewLinkContext(&obj.Link{})
			pos := c.Ctxt.PosTable.XPos(src.MakePos(src.NewFileBase(file, file), 3, 2))
			var log bytes.Buffer
			c.PGOHash = c.NewHashDebug("pgohash", "vy", &log)
			for range 30 {
				if !c.PGOHash.MatchPosWithInfo(pos, "inline", nil) {
					t.Error("match lost")
				}
			}
			if strings.Count(log.String(), file) != 60 {
				t.Errorf("foreign position or lost log: %s", log.String())
			}
			// The hash object retains its owning Link even when another compilation uses
			// exactly the same numeric XPos with a different file.
			other := NewLinkContext(&obj.Link{})
			other.Ctxt.PosTable.XPos(src.MakePos(src.NewFileBase("foreign.go", "foreign.go"), 3, 2))
			if !c.PGOHash.MatchPos(pos, nil) || strings.Contains(log.String(), "foreign.go") {
				t.Error("owner changed")
			}
		})
	}
	wg.Wait()
}
func TestHashDebugExplicitOutput(t *testing.T) {
	path := filepath.Join(t.TempDir(), "host-log")
	t.Setenv("GSHS_LOGFILE", path)
	c := NewLinkContext(&obj.Link{})
	if !c.NewHashDebug("hash", "vy", nil).MatchPkgFunc("p", "F", nil) {
		t.Fatal("nil output changed decision")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("hash logger touched environment path", err)
	}
	if !c.PGOHash.MatchPos(src.XPos{}, nil) || !c.PGOHash.MatchPosWithInfo(src.XPos{}, "inline", nil) {
		t.Fatal("nil hash changed")
	}
	defer func() {
		if recover() == nil {
			t.Error("invalid hash accepted")
		}
	}()
	c.NewHashDebug("hash", "invalid-pattern", nil)
}

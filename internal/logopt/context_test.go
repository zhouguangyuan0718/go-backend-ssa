package logopt

import (
	"fmt"
	"go-backend-ssa/internal/cmd/src"
	"sync"
	"testing"
)

func TestNativeRecordingIsolation(t *testing.T) {
	contexts := make([]*Context, 8)
	var wg sync.WaitGroup
	for i := range contexts {
		contexts[i] = NewContext()
		contexts[i].Format = Json0
		for j := range 16 {
			wg.Go(func() { c := contexts[i]; c.LogOpt(src.XPos{}, "inline", "inline pass", fmt.Sprint(i), j) })
		}
	}
	wg.Wait()
	for i, c := range contexts {
		events := c.Snapshot()
		if len(events) != 16 || !c.Enabled() {
			t.Fatal("lost events")
		}
		for _, e := range events {
			if e.Pass() != "inline_pass" || e.FunctionName() != fmt.Sprint(i) || e.What() != "inline" {
				t.Fatal("event fields")
			}
		}
		old := events[0]
		events[0] = nil
		args := old.Arguments()
		args[0] = "changed"
		if c.Snapshot()[0] != old || old.Arguments()[0] == "changed" {
			t.Fatal("snapshot alias")
		}
		c.LogOptRange(src.XPos{}, src.XPos{}, "range", "walk", "F", "detail", []*LoggedOpt{old})
		if len(events) != 16 || len(c.Snapshot()) != 17 {
			t.Fatal("retained view")
		}
	}
	disabled := NewContext()
	disabled.LogOpt(src.XPos{}, "inline", "inline", "F")
	if disabled.Enabled() || len(disabled.Snapshot()) != 0 {
		t.Fatal("native disabled semantics")
	}
	disabled.Format = 99
	defer func() {
		if recover() == nil {
			t.Fatal("invalid native log mode accepted")
		}
	}()
	disabled.Enabled()
}

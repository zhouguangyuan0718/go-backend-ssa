package objabi

import (
	"fmt"
	"go-backend-ssa/internal/go/bisect"
	"strings"
	"sync"
	"testing"
)

type testDebugFlags struct {
	ConcurrentOk bool
	Number       int             `help:"numeric mode" concurrent:"ok"`
	Text         string          `help:"text mode"`
	Match        *bisect.Matcher `help:"change matcher"`
}

func TestDebugParserNativeSettings(t *testing.T) {
	d := testDebugFlags{ConcurrentOk: true}
	var seen []string
	ssa := func(phase, flag string, val int, text string) string {
		seen = append(seen, fmt.Sprintf("%s/%s:%d:%s", phase, flag, val, text))
		return ""
	}
	help, err := ParseDebugFlags(&d, ssa, []string{"number", "number:7,,", "text=alpha,ssa/check/on,ssa/generic_cse/dump=F,match=n"})
	if err != nil || help != "" || d.Number != 7 || d.Text != "alpha" || d.ConcurrentOk || d.Match == nil || d.Match.ShouldEnable(1) {
		t.Fatal("parsed settings", d, help, err)
	}
	if strings.Join(seen, ",") != "check/on:1:,generic_cse/dump:1:F" {
		t.Fatal("SSA callback", seen)
	}
	untouched := testDebugFlags{ConcurrentOk: true}
	if _, err := ParseDebugFlags(&untouched, nil, []string{"number=2"}); err != nil || !untouched.ConcurrentOk || untouched.Number != 2 {
		t.Fatal("concurrent field", untouched, err)
	}
	if _, err := ParseDebugFlags(&d, nil, []string{"text="}); err != nil || d.Text != "" {
		t.Fatal("empty string setting", d, err)
	}
}

func TestDebugParserFailureAndHelp(t *testing.T) {
	for _, setting := range []string{"unknown=1", "number=bad", "match=???", "ssa/check/on"} {
		t.Run(setting, func(t *testing.T) {
			d := testDebugFlags{ConcurrentOk: true}
			if _, err := ParseDebugFlags(&d, nil, []string{setting}); err == nil {
				t.Fatal("invalid setting accepted")
			}
		})
	}
	d := testDebugFlags{ConcurrentOk: true}
	if _, err := ParseDebugFlags(&d, func(string, string, int, string) string { return "invalid SSA phase" }, []string{"ssa/missing/on"}); err == nil || err.Error() != "invalid SSA phase" {
		t.Fatal("SSA error", err)
	}
	help, err := ParseDebugFlags(&d, func(string, string, int, string) string { return "" }, []string{"help", "number=9"})
	if err != nil || !strings.HasPrefix(help, "usage: -d ") || !strings.Contains(help, "ssa/help") || d.Number != 0 {
		t.Fatal("help did not stop native parser", help, err, d)
	}
	if strings.Index(help, "match") > strings.Index(help, "number") || strings.Index(help, "number") > strings.Index(help, "text") {
		t.Fatal("help order", help)
	}
	// Adapter handles only the parser's explicit process-exit paths.
	func() {
		defer func() {
			if got := recover(); got != "callback bug" {
				t.Fatal("unknown panic lost", got)
			}
		}()
		ParseDebugFlags(&d, func(string, string, int, string) string { panic("callback bug") }, []string{"ssa/check/on"})
	}()
}

func TestDebugParserIndependentCalls(t *testing.T) {
	var wg sync.WaitGroup
	for i := 1; i <= 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			d := testDebugFlags{ConcurrentOk: true}
			_, err := ParseDebugFlags(&d, nil, []string{fmt.Sprintf("number=%d,text=local-%d", i, i)})
			if err != nil || d.Number != i || d.Text != fmt.Sprintf("local-%d", i) {
				t.Error("cross-parser state", d, err)
			}
		}()
	}
	wg.Wait()
}

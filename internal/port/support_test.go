package port

import (
	"context"
	"strings"
	"testing"
)

func TestSupportPackagesCheckedAndRecorded(t *testing.T) {
	const source = `package p;var n int;func Read()int{return n}`
	repo, pin := fixtureRepo(t, map[string]string{"LICENSE": "fixture", "PATENTS": "fixture", "src/cmd/compile/internal/p/p.go": source})
	s, err := OpenSource(context.Background(), repo, pin)
	if err != nil {
		t.Fatal(err)
	}
	plan := Plan{InspectRoots: []string{"cmd/compile/internal/p"}, SupportFiles: []string{"support/support.go"}, Units: []Unit{{Source: "src/cmd/compile/internal/p/p.go", Functions: []string{"Read"}, Context: &ContextRule{Type: "Context", Constructor: "newContext", Adapter: "adapter_context.go", Fields: map[string]string{"n": "n"}}}}}
	dir := t.TempDir()
	writeTestFile(t, dir, "internal/p/adapter_context.go", `package p;import "go-backend-ssa/support";type Context struct{n int;status support.Status}`)
	const good = `package support;type Status struct{Count int}`
	writeTestFile(t, dir, "support/support.go", good)
	r, err := Render(s, plan, dir)
	if err != nil {
		t.Fatal(err)
	}
	if r.Ledger.SupportSources["support/support.go"] != hash([]byte(good)) {
		t.Fatal("support hash missing")
	}
	for _, tc := range []struct{ src, want string }{
		{good + ";var shared int", "remaining package variable"},
		{good + ";func init(){}", "init not supported"},
		{"//go:build linux\n\n" + good, "conditional support"},
		{`package support;import _ "os";type Status struct{}`, "unsupported support import"},
		{`package support;import "go-backend-ssa/support";type Status struct{Parent *support.Status}`, "cyclic support"},
	} {
		writeTestFile(t, dir, "support/support.go", tc.src)
		if _, err := Render(s, plan, dir); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Fatalf("want %q, got %v", tc.want, err)
		}
	}
	writeTestFile(t, dir, "support/support.go", good)
	writeTestFile(t, dir, "support/unreviewed.go", `package support;var Global int`)
	if _, err := Render(s, plan, dir); err == nil || !strings.Contains(err.Error(), "unlisted production") {
		t.Fatalf("unreviewed support source accepted: %v", err)
	}
}

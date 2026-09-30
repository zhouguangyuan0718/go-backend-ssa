package amd64

import (
	"bytes"
	"errors"
	"go-backend-ssa/internal/gc"
	"io"
	"testing"
)

type incompleteExport struct{}

func (incompleteExport) Write(p []byte) (int, error) { return len(p) - 1, nil }

func TestIndependentExportLifecycle(t *testing.T) {
	fresh := func() *gc.Context {
		c, s := loadQueueCompiler(t, 1, "package p;type T struct{X int};func F[T ~int](x T)T{return x+1}", true)
		c.PrepareFrontend(s, nil)
		if _, err := c.BuildDeclarations(s, nil, nil); err != nil {
			t.Fatal(err)
		}
		c.DumpData()
		return c
	}
	c := fresh()
	n := c.Package.Noder
	mustDataPanic(t, func() { _ = n.ExportUnified(new(bytes.Buffer)) })
	if err := n.ExportIndependentUnified(nil); err == nil || !n.ExportAvailable() {
		t.Fatal("invalid sink consumed export")
	}
	var out bytes.Buffer
	if err := n.ExportIndependentUnified(&out); err != nil || out.Len() == 0 || c.Writer.Link.ObjectNumbered() || n.ExportAvailable() {
		t.Fatal("independent export did not consume unnumbered package", err)
	}
	mustDataPanic(t, func() { _ = n.ExportIndependentUnified(new(bytes.Buffer)) })
	failed := fresh().Package.Noder
	if err := failed.ExportIndependentUnified(incompleteExport{}); !errors.Is(err, io.ErrShortWrite) || failed.ExportAvailable() {
		t.Fatal("failed export was not consumed", err)
	}
	mustDataPanic(t, func() { _ = failed.ExportIndependentUnified(new(bytes.Buffer)) })
	after := fresh().Package.Noder
	if err := after.ExportIndependentUnified(new(bytes.Buffer)); err != nil {
		t.Fatal("failure poisoned another context", err)
	}
}

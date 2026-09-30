package amd64

import (
	"bytes"
	"errors"
	"go-backend-ssa/internal/gc"
	"io"
	"testing"
)

type failedHeaderOutput struct{ cause error }

func (o failedHeaderOutput) Write([]byte) (int, error) { return 0, o.cause }

type shortHeaderOutput struct{}

func (shortHeaderOutput) Write([]byte) (int, error)      { return 0, nil }
func (shortHeaderOutput) Seek(int64, int) (int64, error) { return 0, nil }

func (o failedHeaderOutput) Seek(int64, int) (int64, error) { return 0, o.cause }

func TestAssemblyHeaderOutputOwnershipAndFailure(t *testing.T) {
	makeCompiler := func() *gc.Context {
		c, s := loadQueueCompilerWithConfig(t, 1, "package p;const A=19;type Header struct{X byte;Y int64}", true, func(c *gc.Context) { c.Package.Noder.Flag.AsmHdr = "explicit" })
		var early objectOutput
		if e := c.WriteAssemblyHeader(&early); e == nil {
			t.Fatal("uncompiled header accepted")
		}
		c.CompilePackage(s, nil)
		c.SetObjectHeader(gc.ObjectConfig{GOOS: "linux", GOARCH: "amd64", GOAMD64: 1, Version: "go1.27.1"})
		var archive objectOutput
		if e := c.WriteArchive(&archive, gc.CombinedObject); e != nil {
			t.Fatal(e)
		}
		return c
	}
	c := makeCompiler()
	var output archiveTrackedOutput
	if e := c.WriteAssemblyHeader(&output); e != nil {
		t.Fatal(e)
	}
	if output.closed || !bytes.Contains(output.data, []byte("#define Header_Y 8")) {
		t.Fatal("header content/stream ownership", string(output.data), output.closed)
	}
	if e := c.WriteAssemblyHeader(&output); e == nil {
		t.Fatal("header written twice")
	}
	c.CheckPackage()
	broken := makeCompiler()
	if e := broken.WriteAssemblyHeader(failedHeaderOutput{io.ErrClosedPipe}); !errors.Is(e, io.ErrClosedPipe) {
		t.Fatal("lost output error", e)
	}
	if e := broken.WriteAssemblyHeader(&output); e == nil {
		t.Fatal("failed header stage reused")
	}
	if e := makeCompiler().WriteAssemblyHeader(shortHeaderOutput{}); !errors.Is(e, io.ErrShortWrite) {
		t.Fatal("lost short output error", e)
	}
}

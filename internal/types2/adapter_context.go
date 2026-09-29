package types2

import (
	"fmt"
	"io"
)

type contextIO struct{ TraceWriter io.Writer }
type discardTrace struct{}

func (discardTrace) Write(p []byte) (int, error) { return len(p), nil }
func (c *Context) traceWriter() io.Writer {
	if c.TraceWriter != nil {
		return c.TraceWriter
	}
	return discardTrace{}
}
func (c *Context) tracePrintf(format string, args ...any) (int, error) {
	return fmt.Fprintf(c.traceWriter(), format, args...)
}
func (c *Context) tracePrintln(args ...any) (int, error) {
	return fmt.Fprintln(c.traceWriter(), args...)
}
func (c *Context) tracePrint(args ...any) (int, error) { return fmt.Fprint(c.traceWriter(), args...) }

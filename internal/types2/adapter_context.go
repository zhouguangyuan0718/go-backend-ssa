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

// Context returns the compilation instance owning this package's type graph.
func (p *Package) Context() *Context { return p.compilation }

// PackageScope preserves upstream Package.Scope's nil-receiver meaning using
// an explicit compilation universe. A nil Package cannot carry that owner.
func (c *Context) PackageScope(pkg *Package) *Scope {
	if c == nil {
		panic("uninitialized compilation")
	}
	if pkg == nil {
		return c.Universe
	}
	if pkg.compilation != c {
		panic("package belongs to another compilation")
	}
	return pkg.Scope()
}

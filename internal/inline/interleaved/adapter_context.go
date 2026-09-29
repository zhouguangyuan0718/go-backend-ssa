package interleaved

import (
	"fmt"
	"go-backend-ssa/internal/devirtualize"
	"go-backend-ssa/internal/inline"
	"go-backend-ssa/internal/ir"
	"go-backend-ssa/internal/pgoir"
	"io"
)

// Context binds the native fixed-point driver to a single compiler graph.
// Separate contexts may execute concurrently; calls mutating one graph are serial.
type Context struct {
	Inline       *inline.Context
	Devirtualize *devirtualize.Context
}

func NewContext(inl *inline.Context) *Context {
	if inl == nil {
		panic("interleaved optimization requires an inlining context")
	}
	c := newContext()
	c.Inline = inl
	c.Devirtualize = devirtualize.NewContext(inl)
	return c
}
func (c *Context) checkProfile(p *pgoir.Profile) {
	if p != nil && !c.Inline.PGO.OwnsProfile(p) {
		panic("optimization profile belongs to another compilation")
	}
}
func (c *Context) checkFunc(fn *ir.Func, p *pgoir.Profile) {
	if fn == nil || !c.Inline.Typecheck.IR.OwnsNode(fn) {
		panic("optimization function belongs to another compilation")
	}
	c.checkProfile(p)
}
func (c *Context) checkPackage(pkg *ir.Package, p *pgoir.Profile) {
	if pkg == nil || pkg != c.Inline.Typecheck.Target {
		panic("optimization package is not this compilation's target")
	}
	c.checkProfile(p)
	for _, fn := range pkg.Funcs {
		c.checkFunc(fn, p)
	}
}
func (c *Context) traceWriter() io.Writer {
	if w := c.Inline.Typecheck.IR.TraceWriter; w != nil {
		return w
	}
	return io.Discard
}
func (c *Context) tracePrintf(f string, a ...any) (int, error) {
	return fmt.Fprintf(c.traceWriter(), f, a...)
}
func (c *Context) tracePrintln(a ...any) (int, error) { return fmt.Fprintln(c.traceWriter(), a...) }

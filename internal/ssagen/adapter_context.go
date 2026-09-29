package ssagen

import (
	"fmt"
	"go-backend-ssa/diagnostic"
	"go-backend-ssa/internal/cmd/obj"
	"go-backend-ssa/internal/cmd/src"
	errorspkg "go-backend-ssa/internal/go/types/errors"
	"go-backend-ssa/internal/ir"
	"go-backend-ssa/internal/typecheck"
	"go-backend-ssa/internal/types"
	"io"
)

// Context binds native ABI wrapper generation to one compiler graph. Further
// ssagen phases will use the same instance as they are ported.
type Context struct {
	Typecheck *typecheck.Context
	// ReadFile supplies symabis input. There is no implicit filesystem fallback.
	ReadFile func(string) ([]byte, error)
}

func NewContext(tc *typecheck.Context) *Context {
	if tc == nil || tc.IR == nil {
		panic("ABI generation requires a compiler graph")
	}
	c := newContext()
	c.Typecheck = tc
	return c
}
func (c *Context) readFile(name string) ([]byte, error) {
	if c.ReadFile == nil {
		return nil, fmt.Errorf("symabis input reader is not configured")
	}
	return c.ReadFile(name)
}
func (c *Context) fatalLogf(f string, a ...any) { panic(fmt.Errorf(f, a...)) }
func (c *Context) fatalf(f string, a ...any)    { panic(fmt.Errorf(f, a...)) }
func (c *Context) traceWriter() io.Writer {
	if c.Typecheck.IR.TraceWriter != nil {
		return c.Typecheck.IR.TraceWriter
	}
	return io.Discard
}
func (c *Context) errorfAt(pos src.XPos, code errorspkg.Code, f string, a ...any) {
	g := c.Typecheck.IR
	g.Diagnostics.Add(diagnostic.Error, code.String(), g.Base.Ctxt.PosTable.Pos(pos), fmt.Sprintf(f, a...))
}
func (c *Context) checkSym(s *types.Sym) {
	if s == nil || s.Universe() != c.Typecheck.IR.Types {
		panic("ABI symbol belongs to another compilation")
	}
}
func (c *Context) checkWrapper(fn *ir.Func, _ obj.ABI) {
	if fn == nil || !c.Typecheck.IR.OwnsNode(fn) {
		panic("ABI function belongs to another compilation")
	}
}
func (c *Context) checkPackage() {
	if c.Typecheck.Target == nil {
		panic("ABI generation requires a target package")
	}
	for _, fn := range c.Typecheck.Target.Funcs {
		c.checkWrapper(fn, 0)
	}
}

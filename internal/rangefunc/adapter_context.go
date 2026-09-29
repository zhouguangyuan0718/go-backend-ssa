package rangefunc

import (
	"fmt"
	"go-backend-ssa/internal/base"
	"go-backend-ssa/internal/types2"
	"io"
)

// Context supplies native driver inputs and owns the synthetic runtime package.
// Each invocation's AST and checker must belong to this instance's Checker.
type Context struct {
	Checker     *types2.Context
	Flag        base.CmdFlags
	Debug       base.DebugFlags
	TraceWriter io.Writer
	runtimePkg  *types2.Package
}

func NewContext(checker *types2.Context) *Context {
	if checker == nil {
		panic("rangefunc requires checker context")
	}
	c := &Context{Checker: checker}
	c.Debug.RangeFuncCheck = 1
	c.initialize()
	return c
}

func (c *Context) fatalf(format string, args ...any) { panic(fmt.Errorf(format, args...)) }
func (c *Context) traceWriter() io.Writer {
	if c.TraceWriter != nil {
		return c.TraceWriter
	}
	return io.Discard
}

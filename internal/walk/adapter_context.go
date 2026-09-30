package walk

import (
	"fmt"
	"go-backend-ssa/internal/cmd/src"
	"go-backend-ssa/internal/typecheck"
)

// Context binds the selected native walk operations to the compiler graph.
// The full Walk entry point is not yet migrated.
type Context struct {
	Typecheck *typecheck.Context
	// SoftFloat is the native ssagen architecture input, owned by this instance.
	SoftFloat bool
}

func NewContext(tc *typecheck.Context) *Context {
	if tc == nil || tc.IR == nil || tc.IR.Base.Ctxt.Arch == nil {
		panic("walk requires a compiler graph with target architecture")
	}
	c := newContext()
	c.Typecheck = tc
	return c
}
func (c *Context) fatalfAt(pos src.XPos, f string, a ...any) {
	panic(fmt.Errorf("%s: %s", c.Typecheck.IR.Base.FmtPos(pos), fmt.Sprintf(f, a...)))
}

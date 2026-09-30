package coverage

import (
	"fmt"
	"go-backend-ssa/internal/typecheck"
)

// Context binds the native pass to a single compilation. Run before inlining.
type Context struct{ Typecheck *typecheck.Context }

func NewContext(tc *typecheck.Context) *Context {
	if tc == nil || tc.IR == nil || tc.Target == nil {
		panic("coverage requires a compiler graph and package")
	}
	c := newContext()
	c.Typecheck = tc
	return c
}
func (c *Context) checkTarget() {
	for _, n := range c.Typecheck.Target.Externs {
		if n == nil || !c.Typecheck.IR.OwnsNode(n) {
			panic("coverage variable belongs to another compilation")
		}
	}
	for _, fn := range c.Typecheck.Target.Funcs {
		if fn == nil || !c.Typecheck.IR.OwnsNode(fn) {
			panic("coverage function belongs to another compilation")
		}
	}
}

func (c *Context) fatalf(f string, a ...any) { panic(fmt.Errorf(f, a...)) }

package loopvar

import (
	"fmt"
	"go-backend-ssa/diagnostic"
	"go-backend-ssa/internal/cmd/src"
	"go-backend-ssa/internal/ir"
	"go-backend-ssa/internal/logopt"
	"go-backend-ssa/internal/typecheck"
)

// Context binds the native capture rewrite to one IR graph and optimizer log.
// Run ForCapture after package inlining and method wrapper generation. Retain
// the returned records, then LogTransformations after native escape analysis.
type Context struct {
	Typecheck *typecheck.Context
	Log       *logopt.Context
}

func NewContext(tc *typecheck.Context, log *logopt.Context) *Context {
	if tc == nil || tc.IR == nil || log == nil {
		panic("loop capture rewriting requires a compiler graph and optimizer log")
	}
	c := newContext()
	c.Typecheck = tc
	c.Log = log
	return c
}
func (c *Context) checkFunc(fn *ir.Func) {
	if fn == nil || !c.Typecheck.IR.OwnsNode(fn) {
		panic("loop capture function belongs to another compilation")
	}
}
func (c *Context) checkTransformations(records []VarAndLoop) {
	for _, r := range records {
		if r.Name == nil || !c.Typecheck.IR.OwnsNode(r.Name) || r.Loop == nil || !c.Typecheck.IR.OwnsNode(r.Loop) {
			panic("loop capture record belongs to another compilation")
		}
	}
}
func (c *Context) fatalf(f string, a ...any) { panic(fmt.Errorf(f, a...)) }
func (c *Context) warnfAt(pos src.XPos, f string, a ...any) {
	g := c.Typecheck.IR
	g.Diagnostics.Add(diagnostic.Warning, "", g.Base.Ctxt.PosTable.Pos(pos), fmt.Sprintf(f, a...))
}

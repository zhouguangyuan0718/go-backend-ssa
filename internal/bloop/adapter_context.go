package bloop

import (
	"fmt"
	"go-backend-ssa/diagnostic"
	"go-backend-ssa/internal/cmd/src"
	"go-backend-ssa/internal/ir"
	"go-backend-ssa/internal/typecheck"
)

// Context binds the native pass to a single compilation. Run before inlining.
type Context struct{ Typecheck *typecheck.Context }

func NewContext(tc *typecheck.Context) *Context {
	if tc == nil || tc.IR == nil || tc.Target == nil {
		panic("bloop requires a compiler graph and package")
	}
	c := newContext()
	c.Typecheck = tc
	return c
}
func (c *Context) checkPackage(pkg *ir.Package) {
	if pkg == nil || pkg != c.Typecheck.Target {
		panic("bloop package belongs to another compilation")
	}
	for _, fn := range pkg.Funcs {
		if fn == nil || !c.Typecheck.IR.OwnsNode(fn) {
			panic("bloop function belongs to another compilation")
		}
	}
}
func (c *Context) fatalfAt(pos src.XPos, f string, a ...any) {
	panic(fmt.Errorf("%s: %s", c.Typecheck.IR.Base.FmtPos(pos), fmt.Sprintf(f, a...)))
}
func (c *Context) warnfAt(pos src.XPos, f string, a ...any) {
	g := c.Typecheck.IR
	g.Diagnostics.Add(diagnostic.Warning, "", g.Base.Ctxt.PosTable.Pos(pos), fmt.Sprintf(f, a...))
}

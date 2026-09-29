package pkginit

import (
	"fmt"
	"go-backend-ssa/internal/ir"
	"go-backend-ssa/internal/noder"
)

// Context owns package initialization and ASan registration for one compiler
// graph. MakeTask runs once after loopvar and before ABI wrappers and escape.
type Context struct {
	Noder                  *noder.Context
	InstrumentGlobalsMap   map[string]ir.Node
	InstrumentGlobalsSlice []ir.Node
}

func NewContext(reader *noder.Context) *Context {
	c := newContext()
	c.Noder = reader
	c.checkPackage()
	return c
}
func (c *Context) checkPackage() {
	n := c.Noder
	if n == nil || n.Typecheck == nil || n.Typecheck.IR == nil || n.StaticInit == nil || n.ReflectData == nil || n.StaticInit.Typecheck != n.Typecheck || n.StaticInit.ReflectData != n.ReflectData || n.ReflectData.Typecheck != n.Typecheck {
		panic("package initialization requires a shared reader and initialization graph")
	}
	if n.Typecheck.Target == nil {
		panic("package initialization requires a target package")
	}
	for _, fn := range n.Typecheck.Target.Inits {
		if fn == nil || !n.Typecheck.IR.OwnsNode(fn) {
			panic("package init function belongs to another compilation")
		}
	}
	for _, node := range n.Typecheck.Target.Externs {
		if node == nil || !n.Typecheck.IR.OwnsNode(node) {
			panic("package init global belongs to another compilation")
		}
	}
	for _, pkg := range n.Typecheck.Target.Imports {
		if pkg == nil || pkg.Universe() != n.Typecheck.IR.Types {
			panic("package init import belongs to another compilation")
		}
		if sym := pkg.Syms[".inittask"]; sym != nil {
			if task, ok := sym.Def.(*ir.Name); ok && !n.Typecheck.IR.OwnsNode(task) {
				panic("imported init task belongs to another compilation")
			}
		}
	}
}
func (c *Context) fatalf(f string, a ...any) { panic(fmt.Errorf(f, a...)) }

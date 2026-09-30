package gc

import (
	"fmt"
	"go-backend-ssa/internal/ir"
	"go-backend-ssa/internal/objw"
	"go-backend-ssa/internal/pkginit"
)

// Context binds the compiler's sequential object-finalization operations.
// Later driver stages share the same writer and compiler graph.
type Context struct {
	Writer           *objw.Context
	Package          *pkginit.Context
	functionDataDone bool
}

func NewContext(writer *objw.Context) *Context {
	if writer == nil || writer.Link == nil || writer.Types == nil {
		panic("compiler finalization requires an object writer")
	}
	return &Context{Writer: writer}
}

// AddGCLocals runs the native sequential registration of all function auxiliary
// data after Flush and before symbol numbering. Call once per compilation.
func (c *Context) AddGCLocals() {
	if c == nil || c.Writer == nil || c.functionDataDone {
		panic("function data finalization requires an unconsumed compiler context")
	}
	c.functionDataDone = true
	c.addGCLocals()
}

// BindPackage shares the package initialization and reflection graph used by
// global declaration finalization. It must be bound before DumpGlobal.
func (c *Context) BindPackage(p *pkginit.Context) {
	if c == nil || c.Writer == nil || p == nil || p.Noder == nil || p.Noder.Typecheck == nil || p.Noder.Typecheck.IR == nil || p.Noder.ReflectData == nil || p.Noder.ReflectData.Typecheck != p.Noder.Typecheck || p.Noder.ReflectData.Runtime == nil || p.Noder.Typecheck.IR.Types != c.Writer.Types || p.Noder.Typecheck.IR.Base.Ctxt != c.Writer.Link || p.Noder.ReflectData.Runtime.Writer != c.Writer || c.Package != nil {
		panic("global finalization requires the same unbound package graph")
	}
	c.Package = p
}
func (c *Context) checkGlobal(n *ir.Name) {
	if c == nil || c.Package == nil || c.functionDataDone || n == nil || !c.Package.Noder.Typecheck.IR.OwnsNode(n) {
		panic("global belongs to another or finalized compilation")
	}
}

// DumpGlobal emits the native data definition and DWARF for a global name.
func (c *Context) DumpGlobal(n *ir.Name) { c.checkGlobal(n); c.dumpGlobal(n) }

// DumpGlobalConst emits the native typed integer constant DWARF record.
func (c *Context) DumpGlobalConst(n *ir.Name) { c.checkGlobal(n); c.dumpGlobalConst(n) }
func (c *Context) fatalf(f string, a ...any)  { panic(fmt.Errorf(f, a...)) }

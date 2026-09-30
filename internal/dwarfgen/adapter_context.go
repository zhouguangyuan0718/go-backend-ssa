package dwarfgen

import (
	"flag"
	"fmt"
	"go-backend-ssa/internal/base"
	"go-backend-ssa/internal/cmd/dwarf"
	"go-backend-ssa/internal/cmd/obj"
	"go-backend-ssa/internal/cmd/src"
	"go-backend-ssa/internal/ir"
	"go-backend-ssa/internal/reflectdata"
	"go-backend-ssa/internal/ssa"
	"go-backend-ssa/internal/ssagen"
	"io"
)

// Context binds native debug generation to one compiler graph and position table.
// Scope markers may use only Base; emission requires NewContext.
type Context struct {
	Base        *base.LinkContext
	Backend     *ssagen.Context
	ReflectData *reflectdata.Context
	Flags       interface{ Lookup(string) *flag.Flag }
	ptrSize     int
}

// NewContext binds the native compiler callbacks and an owned DWARF encoder.
func NewContext(backend *ssagen.Context, reflectData *reflectdata.Context) *Context {
	if backend == nil || backend.SSA == nil || backend.Typecheck == nil || reflectData == nil || reflectData.Typecheck != backend.Typecheck || backend.SSA.IR != backend.Typecheck.IR || backend.SSA.ReflectData != reflectData {
		panic("DWARF generation requires one compiler graph")
	}
	g := backend.Typecheck.IR
	if g.Base.Ctxt.DWARF != nil {
		panic("DWARF generation is already configured")
	}
	c := &Context{Base: g.Base, Backend: backend, ReflectData: reflectData, ptrSize: g.Types.PointerSize()}
	link := g.Base.Ctxt
	link.EnsureDwarfFixups()
	link.DWARF = dwarf.NewEmitter(link.TargetOS, g.Types.Experiment)
	link.DWARF.TraceWriter = g.TraceWriter
	link.DebugInfo = c.Info
	link.GenAbstractFunc = c.AbstractFunc
	return c
}
func (c *Context) checkInfo(link *obj.Link, fn, info *obj.LSym, curfn obj.Func) {
	f, ok := curfn.(*ir.Func)
	if c.Backend == nil || link != c.Base.Ctxt || !ok || ir.ContextOf(f) != c.Backend.Typecheck.IR || fn == nil || info == nil {
		panic("DWARF information requires an owned function and Link")
	}
	if f.DebugInfo != nil {
		_ = c.Backend.SSA.BindFuncDebug(*f.DebugInfo.(*ssa.FuncDebug))
	}
	for _, n := range f.Dcl {
		if !c.Backend.Typecheck.IR.OwnsNode(n) {
			panic("DWARF declaration belongs to another compilation")
		}
	}
}
func (c *Context) checkFlags(flags ...string) {
	if c.Flags == nil {
		panic("DWARF flags require an explicit flag set")
	}
}
func (c *Context) lookupFlag(name string) *flag.Flag { c.checkFlags(); return c.Flags.Lookup(name) }
func (c *Context) traceWriter() io.Writer {
	if c.Backend != nil && c.Backend.Typecheck.IR.TraceWriter != nil {
		return c.Backend.Typecheck.IR.TraceWriter
	}
	return io.Discard
}
func (c *Context) tracePrintf(f string, a ...any) (int, error) {
	return fmt.Fprintf(c.traceWriter(), f, a...)
}
func (c *Context) tracePrint(a ...any) (int, error)   { return fmt.Fprint(c.traceWriter(), a...) }
func (c *Context) tracePrintln(a ...any) (int, error) { return fmt.Fprintln(c.traceWriter(), a...) }

func NewScopeMarker(base *base.LinkContext) ScopeMarker {
	if base == nil || base.Ctxt == nil {
		panic("scope marker requires a compiler position table")
	}
	return ScopeMarker{compilation: &Context{Base: base}}
}
func (c *Context) fatalf(format string, args ...any) { panic(fmt.Errorf(format, args...)) }
func (c *Context) fatalfAt(pos src.XPos, format string, args ...any) {
	panic(fmt.Errorf("%s: %s", c.Base.FmtPos(pos), fmt.Sprintf(format, args...)))
}

// Bind attaches a zero marker to the existing compiler position table. Rebinding
// the same table preserves recorded scopes; a different table is rejected.
func (m *ScopeMarker) Bind(base *base.LinkContext) {
	if base == nil || base.Ctxt == nil {
		panic("scope marker requires a compiler position table")
	}
	if m.compilation != nil {
		if m.compilation.Base != base {
			panic("scope marker belongs to another compilation")
		}
		return
	}
	m.compilation = &Context{Base: base}
}

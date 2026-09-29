package staticinit

import (
	"fmt"
	"go-backend-ssa/diagnostic"
	"go-backend-ssa/internal/cmd/src"
	"go-backend-ssa/internal/ir"
	"go-backend-ssa/internal/reflectdata"
	"go-backend-ssa/internal/typecheck"
	"go-backend-ssa/internal/types"
	"io"
)

// Context owns native static-initialization counters and map-init registration.
// Operations within one compiler graph are serial; independent graphs may run
// concurrently. Schedules and emitted symbols retain their original graph.
type Context struct {
	Typecheck               *typecheck.Context
	ReflectData             *reflectdata.Context
	ptrSize                 int
	maxWidth                int64
	varToMapInit            map[*ir.Name]*ir.Func
	MapInitToVar            map[*ir.Func]*ir.Name
	statuniqgen, mapinitgen int
}

func NewContext(tc *typecheck.Context, rd *reflectdata.Context) *Context {
	if tc == nil || rd == nil || rd.Typecheck != tc || tc.IR == nil {
		panic("static initialization requires shared typecheck and reflection data")
	}
	c := newContext()
	c.Typecheck = tc
	c.ReflectData = rd
	c.ptrSize = tc.IR.Types.PointerSize()
	c.maxWidth = tc.IR.Types.MaxWidth()
	return c
}

// NewSchedule constructs the original Schedule with its native per-schedule
// maps and this compilation's state. Use a fresh schedule for each init function.
func (c *Context) NewSchedule() *Schedule {
	return &Schedule{compilation: c, Plans: make(map[ir.Node]*Plan), Temps: make(map[ir.Node]*ir.Name)}
}
func (c *Context) fatalf(f string, a ...any) { panic(fmt.Errorf(f, a...)) }
func (c *Context) fatalfAt(pos src.XPos, f string, a ...any) {
	panic(fmt.Errorf("%s: %s", c.Typecheck.IR.Base.FmtPos(pos), fmt.Sprintf(f, a...)))
}
func (c *Context) warnfAt(pos src.XPos, f string, a ...any) {
	c.Typecheck.IR.Diagnostics.Add(diagnostic.Warning, "", c.Typecheck.IR.Base.Ctxt.PosTable.Pos(pos), fmt.Sprintf(f, a...))
}
func (c *Context) traceWriter() io.Writer {
	if c.Typecheck.IR.TraceWriter != nil {
		return c.Typecheck.IR.TraceWriter
	}
	return io.Discard
}
func (c *Context) checkType(t *types.Type) {
	if t == nil || t.Universe() != c.Typecheck.IR.Types {
		panic("static initialization type belongs to another compilation")
	}
}
func (c *Context) checkFunc(fn *ir.Func) {
	if fn == nil || fn.Sym() == nil || fn.Sym().Universe() != c.Typecheck.IR.Types {
		panic("static initialization function belongs to another compilation")
	}
}
func (c *Context) checkAssign(l *ir.Name, off int64, r ir.Node, t *types.Type) {
	c.checkType(t)
	if l == nil || l.Sym() == nil || l.Sym().Universe() != c.Typecheck.IR.Types {
		panic("static initialization name belongs to another compilation")
	}
	if r != nil && r.Type() != nil {
		c.checkType(r.Type())
	}
}

package staticdata

import (
	"fmt"
	"go-backend-ssa/diagnostic"
	"go-backend-ssa/internal/cmd/obj"
	"go-backend-ssa/internal/cmd/src"
	errors "go-backend-ssa/internal/go/types/errors"
	"go-backend-ssa/internal/ir"
	"go-backend-ssa/internal/objw"
	"io/fs"
	"sync"
)

// Context owns native static-data counters and the deferred function-value
// queue. It shares the existing IR and object writer of one compilation.
// Serialize data writes; FuncLinksym may register concurrently after the input
// graph is frozen, with WriteFuncSyms called only after registrations finish.
type Context struct {
	// OpenFile supplies immutable input bytes and metadata. Large inputs are
	// reopened by Writer.Link.OpenObjectInput during object serialization.
	OpenFile     func(string) (fs.File, error)
	IR           *ir.Context
	Writer       *objw.Context
	ptrSize      int
	slicedataGen int
	funcsymsmu   sync.Mutex
	funcsyms     []*ir.Name
}

func NewContext(graph *ir.Context, writer *objw.Context) *Context {
	if graph == nil || graph.Base == nil || writer == nil || graph.Types != writer.Types || graph.Base.Ctxt != writer.Link {
		panic("static data requires IR and object writer from the same compilation")
	}
	c := newContext()
	c.IR, c.Writer, c.ptrSize = graph, writer, graph.Types.PointerSize()
	return c
}
func (c *Context) fatalf(f string, a ...any) { panic(fmt.Errorf(f, a...)) }
func (c *Context) errorfAt(pos src.XPos, code errors.Code, f string, a ...any) {
	c.IR.Diagnostics.Add(diagnostic.Error, code.String(), c.Writer.Link.PosTable.Pos(pos), fmt.Sprintf(f, a...))
}
func (c *Context) checkName(n *ir.Name) {
	if n == nil || n.Sym() == nil || n.Sym().Universe() != c.IR.Types || n.Type() != nil && n.Type().Universe() != c.IR.Types {
		panic("static data name belongs to another or uninitialized compilation")
	}
}
func (c *Context) checkAddr(n *ir.Name, noff int64, sym *obj.LSym, off int64) {
	c.checkName(n)
}
func (c *Context) checkSlice(n *ir.Name, noff int64, sym *obj.LSym, length int64) {
	c.checkName(n)
}
func (c *Context) checkSliceBytes(n *ir.Name, off int64, s string) { c.checkName(n) }
func (c *Context) checkConst(n *ir.Name, off int64, value ir.Node, width int) {
	c.checkName(n)
	if value == nil || value.Type() == nil || value.Type().Universe() != c.IR.Types {
		panic("static constant belongs to another or uninitialized compilation")
	}
}

func (c *Context) openFile(path string) (fs.File, error) {
	if c.OpenFile == nil {
		return nil, fmt.Errorf("static data file provider is not configured: %s", path)
	}
	return c.OpenFile(path)
}
func (c *Context) errors() int {
	n := 0
	for _, ds := range [][]diagnostic.Diagnostic{c.IR.Diagnostics.Diagnostics(), c.IR.Types.Diagnostics.Diagnostics()} {
		for _, d := range ds {
			if d.Severity == diagnostic.Error {
				n++
			}
		}
	}
	return n
}
func (c *Context) checkEmbed(n *ir.Name) {
	c.checkName(n)
	if n.Type() == nil || n.Embed == nil || len(*n.Embed) == 0 || c.Writer.Link.ObjectNumbered() {
		panic("embed requires an owned typed declaration before object numbering")
	}
}

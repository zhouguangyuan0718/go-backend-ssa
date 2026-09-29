package typecheck

import (
	"fmt"
	"go-backend-ssa/diagnostic"
	"go-backend-ssa/internal/cmd/src"
	errorspkg "go-backend-ssa/internal/go/types/errors"
	"go-backend-ssa/internal/ir"
	"go-backend-ssa/internal/types"
	"io"
	"sync"
)

// Context owns native typecheck tables and uses the existing compiler instance
// for IR, symbols, types, current function and current position. Its mutable
// operations and those of IR must be serialized together.
type Context struct {
	Target         *ir.Package
	HaveInlineBody func(fn *ir.Func) bool
	typeSymIdx     map[*types.Type][2]int64
	funcStack      []*ir.Func
	autotmpnamesmu sync.Mutex
	autotmpnames   []string
	dotlist        []dlist
	slist          []symlink
	traceIndent    []byte
	ptrSize        int
	lastError      diagnostic.Diagnostic
	lastErrorLine  string

	runtimeDecls [228]struct {
		name     string
		tag, typ int
	}
	coverageDecls [1]struct {
		name     string
		tag, typ int
	}

	IR                                                                                *ir.Context
	okfor                                                                             [ir.OEND][]bool
	okforeq, okforadd, okforand, okfornone, okforbool, okforcap, okforlen, okforarith [types.NTYPE]bool
	builtinFuncs                                                                      [18]struct {
		name string
		op   ir.Op
	}
	unsafeFuncs [5]struct {
		name string
		op   ir.Op
	}
}

// NewContext creates native typecheck state; call InitUniverse exactly once on
// a fresh compiler Universe, before constructing checked IR or reading packages.
func NewContext(graph *ir.Context) *Context {
	if graph == nil || graph.Types == nil || graph.Base == nil || graph.Base.Ctxt == nil || graph.Types.PosTable != &graph.Base.Ctxt.PosTable {
		panic("typecheck requires an initialized IR context")
	}
	c := &Context{IR: graph, Target: &ir.Package{}, ptrSize: graph.Types.PointerSize()}
	c.initialize()
	return c
}
func (c *Context) fatalf(format string, args ...any) { panic(fmt.Errorf(format, args...)) }

func (c *Context) errorf(format string, args ...any) { c.errorfAt(c.IR.Types.Pos, 0, format, args...) }
func (c *Context) errorfAt(pos src.XPos, code errorspkg.Code, format string, args ...any) {
	c.IR.Diagnostics.Add(diagnostic.Error, code.String(), c.IR.Base.Ctxt.PosTable.Pos(pos), fmt.Sprintf(format, args...))
	entries := c.IR.Diagnostics.Diagnostics()
	c.lastError = entries[len(entries)-1]
	c.lastErrorLine = c.IR.Base.FmtPos(pos)
}
func (c *Context) errors() int {
	n := 0
	for _, entries := range [][]diagnostic.Diagnostic{c.IR.Diagnostics.Diagnostics(), c.IR.Types.Diagnostics.Diagnostics()} {
		for _, d := range entries {
			if d.Severity == diagnostic.Error {
				n++
			}
		}
	}
	return n
}

// Preserve the native selector-error refinement on structured diagnostics. The
// compare-and-rewrite also prevents changing an intervening phase's message.
func (c *Context) updateErrorDot(line, name, expr string) {
	if c.lastErrorLine == line && c.lastError.Message == fmt.Sprintf("undefined: %v", name) {
		c.IR.Diagnostics.RewriteLast(c.lastError, fmt.Sprintf("undefined: %v in %v", name, expr))
	}
}
func (c *Context) fatalfAt(pos src.XPos, format string, args ...any) {
	panic(fmt.Errorf("%s: %s", c.IR.Base.FmtPos(pos), fmt.Sprintf(format, args...)))
}
func (c *Context) assertf(ok bool, format string, args ...any) {
	if !ok {
		c.fatalf(format, args...)
	}
}
func (c *Context) assertfAt(ok bool, pos src.XPos, format string, args ...any) {
	if !ok {
		c.fatalfAt(pos, format, args...)
	}
}
func (c *Context) tracePrintf(format string, args ...any) (int, error) {
	w := c.IR.TraceWriter
	if w == nil {
		w = io.Discard
	}
	return fmt.Fprintf(w, format, args...)
}

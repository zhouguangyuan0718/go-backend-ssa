package typecheck

import (
	"fmt"
	"go-backend-ssa/internal/ir"
	"go-backend-ssa/internal/types"
)

// Context owns native typecheck tables and uses the existing compiler instance
// for IR, symbols, types, current function and current position. Its mutable
// operations and those of IR must be serialized together.
type Context struct {
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
	c := &Context{IR: graph}
	c.initialize()
	return c
}
func (c *Context) fatalf(format string, args ...any) { panic(fmt.Errorf(format, args...)) }

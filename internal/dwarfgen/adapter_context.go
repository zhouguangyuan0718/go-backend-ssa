package dwarfgen

import (
	"fmt"
	"go-backend-ssa/internal/base"
	"go-backend-ssa/internal/cmd/src"
)

// Context shares the compiler's existing position namespace. It has no global
// target or current function state.
type Context struct{ Base *base.LinkContext }

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

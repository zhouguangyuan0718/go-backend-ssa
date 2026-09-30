package slice

import (
	"go-backend-ssa/internal/escape"
	"go-backend-ssa/internal/ir"
	"go-backend-ssa/internal/reflectdata"
)

// Context binds the native slice backing-store optimization to the existing
// escape analysis and runtime type inputs. Run Funcs after escape analysis.
type Context struct {
	Escape      *escape.Context
	ReflectData *reflectdata.Context
}

func NewContext(esc *escape.Context, rd *reflectdata.Context) *Context {
	if esc == nil || esc.Typecheck == nil || esc.Typecheck.IR == nil || rd == nil || rd.Typecheck != esc.Typecheck {
		panic("slice optimization requires escape and reflection inputs from the same compilation")
	}
	c := newContext()
	c.Escape = esc
	c.ReflectData = rd
	return c
}
func (c *Context) checkFuncs(fns []*ir.Func) {
	for _, fn := range fns {
		if fn == nil || !c.Escape.Typecheck.IR.OwnsNode(fn) {
			panic("slice function belongs to another compilation")
		}
	}
}

package reflectdata

import (
	"fmt"
	"go-backend-ssa/internal/cmd/obj"
	"go-backend-ssa/internal/cmd/src"
	"go-backend-ssa/internal/rttype"
	"go-backend-ssa/internal/typecheck"
	"go-backend-ssa/internal/types"
	"sync"
)

// Context owns the native runtime-type emission queue, retaining the same IR,
// types, symbols and target writer as the rest of this compiler instance.
// Serialize use with the attached typecheck context and runtime layouts.
type Context struct {
	ptrSize                                              int
	cachedMapTableType, cachedMapType, cachedMapIterType *types.Type
	Typecheck                                            *typecheck.Context
	Runtime                                              *rttype.Context
	signatmu                                             sync.Mutex
	signatset                                            map[*types.Type]struct{}
	signatslice                                          []typeAndStr
}

func NewContext(tc *typecheck.Context, rt *rttype.Context) *Context {
	if tc == nil || tc.IR == nil || tc.IR.Base == nil || rt == nil || rt.Type == nil || rt.Writer == nil || tc.IR.Types != rt.Types || tc.IR.Base.Ctxt != rt.Writer.Link {
		panic("reflection data requires typecheck and runtime layouts from the same compilation")
	}
	c := newContext()
	c.Typecheck, c.Runtime, c.ptrSize = tc, rt, rt.Types.PointerSize()
	return c
}
func (c *Context) fatalf(f string, a ...any) { panic(fmt.Errorf(f, a...)) }
func (c *Context) assertfAt(ok bool, pos src.XPos, f string, a ...any) {
	if !ok {
		panic(fmt.Errorf("%s: %s", c.Typecheck.IR.Base.FmtPos(pos), fmt.Sprintf(f, a...)))
	}
}
func (c *Context) checkType(t *types.Type) {
	if t == nil || t.Universe() != c.Typecheck.IR.Types {
		panic("reflection type belongs to another or uninitialized compilation")
	}
}
func (c *Context) checkPrefix(prefix string, t *types.Type) { c.checkType(t) }
func (c *Context) checkTypeAt(pos src.XPos, t *types.Type)  { c.checkType(t) }
func (c *Context) checkFrom(t *types.Type, from *obj.LSym) {
	c.checkType(t)
	if from == nil {
		panic("nil interface-use source symbol")
	}
}
func (c *Context) checkField(t *types.Type, f *types.Field) {
	c.checkType(t)
	if f == nil || f.Sym == nil || f.Sym.Universe() != c.Typecheck.IR.Types {
		panic("reflection field belongs to another or uninitialized compilation")
	}
}

// Upstream makefield uses a nil Pkg receiver for the anonymous package. Supply
// its existing instance explicitly, retaining the original Field construction.
func (c *Context) makefield(name string, t *types.Type) *types.Field {
	sym, err := c.Typecheck.IR.Types.Lookup(nil, name)
	if err != nil {
		panic(err)
	}
	return c.Typecheck.IR.Types.NewField(src.XPos{}, sym, t)
}

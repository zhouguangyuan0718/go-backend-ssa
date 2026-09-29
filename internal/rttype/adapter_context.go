package rttype

import (
	"fmt"
	"go-backend-ssa/internal/cmd/obj"
	"go-backend-ssa/internal/objw"
	"go-backend-ssa/internal/types"
)

// Context owns the native target layouts. Calls within an instance, its writer
// and its type Universe are serialized; independent instances can run in parallel.
type Context struct {
	Types                                                        *types.Universe
	Writer                                                       *objw.Context
	ptrSize                                                      int
	initialized                                                  bool
	Type, ArrayType, ChanType, FuncType, InterfaceType, MapType  *types.Type
	PtrType, SliceType, StructType, IMethod, Method, StructField *types.Type
	UncommonType, InterfaceSwitch, TypeAssert, ITab              *types.Type
}

// NewContext binds to an existing initialized compiler Universe and writer.
// Call Init once to construct the native runtime layouts for this target.
func NewContext(writer *objw.Context) *Context {
	if writer == nil || writer.Types == nil || writer.Link == nil || writer.Types.Types[types.TUINTPTR] == nil || writer.Types.PosTable != &writer.Link.PosTable {
		panic("runtime layouts require initialized compiler types and object writer")
	}
	c := newContext()
	c.Types, c.Writer, c.ptrSize = writer.Types, writer, writer.Types.PointerSize()
	return c
}
func (c *Context) fatalf(format string, args ...any) { panic(fmt.Errorf(format, args...)) }
func (c *Context) checkInit() {
	if c.initialized {
		panic("runtime layouts already initialized")
	}
	c.initialized = true
}
func (c *Context) checkCursor(s *obj.LSym, off int64, typ *types.Type) {
	if s == nil || typ == nil || typ.Universe() != c.Types {
		panic("runtime cursor requires a symbol and a type from this compilation")
	}
}
func (c *Context) checkArrayCursor(s *obj.LSym, off int64, typ *types.Type, n int) {
	c.checkCursor(s, off, typ)
}
func arrayContext(a ArrayCursor) *Context {
	if a.c.compilation == nil {
		panic("uninitialized runtime array cursor")
	}
	return a.c.compilation
}

// The native reflector allocates fresh, package-less field symbols. Bind just
// these new symbols to the Universe's anonymous package before native NewStruct
// validates them. Retain their identity and leave other ownership checks intact.
func (c *Context) newStruct(fields []*types.Field) *types.Type {
	for _, f := range fields {
		if f == nil || f.Type == nil || f.Type.Universe() != c.Types || f.Sym == nil || f.Sym.Pkg != nil {
			panic("unexpected reflected field ownership")
		}
	}
	sym, err := c.Types.Lookup(nil, "")
	if err != nil {
		panic(err)
	}
	for _, f := range fields {
		f.Sym.Pkg = sym.Pkg
	}
	return c.Types.NewStruct(fields)
}

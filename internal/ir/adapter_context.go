package ir

import (
	"encoding/base64"
	"fmt"
	"go-backend-ssa/diagnostic"
	"go-backend-ssa/internal/base"
	"go-backend-ssa/internal/cmd/obj"
	"go-backend-ssa/internal/cmd/src"
	errors "go-backend-ssa/internal/go/types/errors"
	"go-backend-ssa/internal/types"
	"go/constant"
	"io"
	"reflect"
	"strings"
)

// Context owns native IR state. A context and its graph are used serially;
// independent contexts share no mutable compiler state.
type Context struct {
	Syms symsStruct
	Pkgs struct{ Go, Itab, Runtime, InternalMaps, Coverage *types.Pkg }

	Types                                                         *types.Universe
	Base                                                          *base.LinkContext
	Debug                                                         base.DebugFlags
	CurFunc                                                       *Func
	BlankNode                                                     *Name
	globClosgen                                                   int32
	IsIntrinsicCall                                               func(*CallExpr) bool
	IsIntrinsicSym                                                func(*types.Sym) bool
	EscFmt                                                        func(n Node) string
	nodeType                                                      reflect.Type
	intZero, floatZero, complexZero, intOne, floatOne, complexOne constant.Value
	OKForConst                                                    [types.NTYPE]bool
	MaxStackVarSize, MaxImplicitStackVarSize, MaxSmallArraySize   int64
	TraceWriter                                                   io.Writer
	Diagnostics                                                   diagnostic.Collector
}

func NewContext(u *types.Universe, link *obj.Link) *Context {
	if u == nil || link == nil {
		panic("IR requires compiler types and Link")
	}
	if u.PosTable != &link.PosTable {
		panic("IR and compiler types require the same position table from construction")
	}
	if link.Arch != nil && link.Arch.Arch != nil && (link.Arch.PtrSize != u.PointerSize() || link.Arch.RegSize != u.RegisterSize()) {
		panic("IR target layout mismatch")
	}
	u.BindLink(link)
	c := &Context{Types: u, Base: u.LinkContext()}
	c.initialize()
	// Native driver pseudo-packages from gc/main.go. Keep their source identity
	// separate from real imported packages while using the original link prefixes.
	c.Pkgs.Runtime = u.NewPkg("go.runtime", "runtime")
	c.Pkgs.Runtime.Prefix = "runtime"
	c.Pkgs.InternalMaps = u.NewPkg("go.internal/runtime/maps", "internal/runtime/maps")
	c.Pkgs.InternalMaps.Prefix = "internal/runtime/maps"
	c.Pkgs.Itab = u.NewPkg("go.itab", "go.itab")
	c.Pkgs.Itab.Prefix = "go:itab"
	c.Pkgs.Go = u.NewPkg("go", "")
	c.Pkgs.Coverage = u.NewPkg("go.coverage", "runtime/coverage")
	c.Pkgs.Coverage.Prefix = "runtime/coverage"
	return c
}

func (n *miniNode) compilationContext() *Context { return n.compilation }
func nodeContext(n Node) *Context {
	if n == nil {
		c := new(Context)
		c.initialize()
		return c
	}
	c := n.(interface{ compilationContext() *Context }).compilationContext()
	if c == nil {
		panic("IR node has no compiler context")
	}
	return c
}
func nodesContext(nodes Nodes) *Context {
	var c *Context
	for _, n := range nodes {
		if n != nil {
			owner := nodeContext(n)
			if c != nil && c != owner {
				panic("mixed IR contexts")
			}
			c = owner
		}
	}
	if c == nil {
		return nodeContext(nil)
	}
	return c
}

// The Op predicate has no state; only its invalid-op diagnostic needs a receiver.
func opContext(Op) *Context { return &Context{} }

func (c *Context) fatalf(format string, args ...any) { panic(fmt.Errorf(format, args...)) }
func (c *Context) fatalfAt(pos src.XPos, format string, args ...any) {
	panic(fmt.Errorf("%s: %s", c.Base.FmtPos(pos), fmt.Sprintf(format, args...)))
}
func (c *Context) errorfAt(pos src.XPos, code errors.Code, format string, args ...any) {
	c.Diagnostics.Add(diagnostic.Error, code.String(), c.Base.Ctxt.PosTable.Pos(pos), fmt.Sprintf(format, args...))
}
func (c *Context) warnfAt(pos src.XPos, format string, args ...any) {
	c.Diagnostics.Add(diagnostic.Warning, "", c.Base.Ctxt.PosTable.Pos(pos), fmt.Sprintf(format, args...))
}
func (c *Context) warn(format string, args ...any) { c.warnfAt(c.Types.Pos, format, args...) }
func (c *Context) traceWriter() io.Writer {
	if c.TraceWriter != nil {
		return c.TraceWriter
	}
	return io.Discard
}
func (c *Context) tracePrintf(format string, args ...any) (int, error) {
	return fmt.Fprintf(c.traceWriter(), format, args...)
}

// Native obj.TrimInlineHash algorithm, with the immutable package initializer
// localized. Keep in sync with src/cmd/internal/obj/objfile.go at the pinned Go.
func (c *Context) trimInlineHash(name string) string {
	inlHashLen := base64.StdEncoding.EncodedLen(8)
	a, b, ok := strings.Cut(name, "#")
	if !ok {
		return name
	}
	if len(b) < inlHashLen+1 || b[inlHashLen] != '#' {
		return name
	}
	return a + b[inlHashLen+1:]
}

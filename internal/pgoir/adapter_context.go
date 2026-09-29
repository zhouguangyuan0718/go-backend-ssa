package pgoir

import (
	"fmt"
	"go-backend-ssa/internal/cmd/pgo"
	"go-backend-ssa/internal/ir"
	"go-backend-ssa/internal/typecheck"
	"io"
	"maps"
	"slices"
)

// Context binds native profile attribution to an existing compiler graph.
// Independent contexts may run concurrently; operations on one graph are serial.
type Context struct {
	Typecheck *typecheck.Context
	// These are the upstream cycle-breaking hooks, now owned by this compilation.
	// The frontend must bind both before building a nonempty profile. Unbound
	// hooks retain upstream's fatal behavior; there is no successful fallback.
	LookupFunc        func(fullName string) (*ir.Func, error)
	PostLookupCleanup func()
}

func NewContext(tc *typecheck.Context) *Context {
	if tc == nil || tc.IR == nil || tc.Target == nil {
		panic("PGO attribution requires a compiler graph")
	}
	c := newContext()
	c.Typecheck = tc
	return c
}

// NewProfile attributes already processed native profile data. It replaces only
// the filesystem/parsing shell of upstream New; createIRGraph is migrated code.
// The input maps are copied so a returned graph retains its own profile data.
// The caller must not mutate input while this call is running.
func (c *Context) NewProfile(input *pgo.Profile) *Profile {
	if input == nil || input.TotalWeight == 0 {
		return nil
	}
	for _, fn := range c.Typecheck.Target.Funcs {
		c.checkNode(fn)
	}
	data := *input
	data.NamedEdgeMap.Weight = maps.Clone(input.NamedEdgeMap.Weight)
	data.NamedEdgeMap.ByWeight = slices.Clone(input.NamedEdgeMap.ByWeight)
	g := c.createIRGraph(data.NamedEdgeMap)
	for _, node := range g.IRNodes {
		if node.AST != nil {
			c.checkNode(node.AST)
		}
	}
	return &Profile{compilation: c, Profile: &data, WeightedCG: g}
}

func (c *Context) checkNode(n ir.Node) {
	if n == nil {
		panic("PGO attribution requires an IR node")
	}
	if typ := n.Type(); typ != nil && typ.Universe() != c.Typecheck.IR.Types {
		panic("PGO node belongs to another compilation")
	}
	if sym := n.Sym(); sym != nil && sym.Universe() != c.Typecheck.IR.Types {
		panic("PGO symbol belongs to another compilation")
	}
}
func (c *Context) checkOffset(n ir.Node, fn *ir.Func) { c.checkNode(n); c.checkNode(fn) }
func (c *Context) fatalf(f string, a ...any)          { panic(fmt.Errorf(f, a...)) }
func (c *Context) traceWriter() io.Writer {
	if w := c.Typecheck.IR.TraceWriter; w != nil {
		return w
	}
	return io.Discard
}

func (c *Context) tracePrintf(f string, a ...any) (int, error) {
	return fmt.Fprintf(c.traceWriter(), f, a...)
}

// OwnsProfile reports whether the attributed IR graph belongs to this instance.
func (c *Context) OwnsProfile(p *Profile) bool { return p != nil && p.compilation == c }

package objw

import (
	"fmt"
	"go-backend-ssa/diagnostic"
	"go-backend-ssa/internal/cmd/obj"
	"go-backend-ssa/internal/ir"
	"go-backend-ssa/internal/types"
)

// Context binds the native writer to an existing compiler Universe and Link.
// Callers serialize mutation of a Context and its symbols.
type Context struct {
	IR              *ir.Context
	sharedProgArray *[10000]obj.Prog
	Types           *types.Universe
	Link            *obj.Link
	ptrSize         int
}

func NewContext(u *types.Universe, link *obj.Link, goos string) *Context {
	if u == nil || link == nil || link.Arch == nil || link.Arch.Arch == nil || goos == "" || link.Arch.ByteOrder == nil {
		panic("object writer requires an explicit target and byte order")
	}
	if u.PosTable != &link.PosTable || link.Arch.PtrSize != u.PointerSize() || link.Arch.RegSize != u.RegisterSize() {
		panic("object writer target/position mismatch")
	}
	if link.TargetOS != "" && link.TargetOS != goos {
		panic("object writer cannot change the target OS")
	}
	u.BindLink(link)
	link.TargetOS = goos
	c := newContext()
	c.Types = u
	c.Link = link
	c.ptrSize = u.PointerSize()
	return c
}
func (c *Context) fatalf(format string, args ...any) { panic(fmt.Errorf(format, args...)) }

func (c *Context) checkNewProgs(fn *ir.Func, worker int) {
	g := ir.ContextOf(fn)
	if g == nil || g.Types != c.Types || g.Base.Ctxt != c.Link || worker < 0 || worker >= g.Base.Flag.LowerC {
		panic("instruction writer requires an owned function and valid worker")
	}
	if c.IR != nil && c.IR != g {
		panic("instruction writer graph mismatch")
	}
	c.IR = g
}
func (c *Context) warn(format string, args ...any) {
	c.IR.Diagnostics.Add(diagnostic.Warning, "", c.Link.OutermostPos(c.Types.Pos), fmt.Sprintf(format, args...))
}

// OwnsProgs reports whether pp is an active instruction buffer of this writer.
func (c *Context) OwnsProgs(pp *Progs) bool {
	return c != nil && pp != nil && pp.compilation == c && pp.CurFunc != nil && pp.Text != nil && pp.Text.Ctxt == c.Link
}

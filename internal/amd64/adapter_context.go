package amd64

import (
	"fmt"
	"go-backend-ssa/diagnostic"
	"go-backend-ssa/internal/cmd/obj"
	"go-backend-ssa/internal/cmd/obj/x86"
	"go-backend-ssa/internal/cmd/src"
	"go-backend-ssa/internal/cmd/sys"
	"go-backend-ssa/internal/ssa"
	"go-backend-ssa/internal/ssagen"
)

// Context binds native AMD64 instruction generation to a compiler instance.
// Initialize the target before ssagen.InitConfig. Assembly is a later operation.
type Context struct {
	x86                *x86.Context
	Backend            *ssagen.Context
	LinkAMD64          obj.LinkArch
	ptrSize            int
	leaptr             obj.As
	blockJump          [ssa.BlockAMD64NAN + 1]struct{ asm, invasm obj.As }
	eqfJumps, nefJumps [2][2]ssagen.IndexJump
}

func NewContext(b *ssagen.Context) *Context {
	if b == nil || b.Typecheck == nil || b.Typecheck.IR == nil {
		panic("AMD64 emission requires a compiler graph")
	}
	if b.SSAConfig() != nil {
		panic("initialize AMD64 handlers before SSA configuration")
	}
	link := b.Typecheck.IR.Base.Ctxt
	if link == nil || link.Arch == nil || link.Arch.Arch == nil || link.Arch.Family != sys.AMD64 || b.Typecheck.IR.Types.PointerSize() != 8 {
		panic("AMD64 emission requires an AMD64 target")
	}
	c := &Context{Backend: b, LinkAMD64: *link.Arch, ptrSize: 8}
	c.initialize()
	c.x86 = x86.NewContext(link)
	c.LinkAMD64.DWARFRegisters = c.x86.AMD64DWARFRegisters
	c.Init(&b.Arch)
	link.Arch = b.Arch.LinkArch
	return c
}
func (c *Context) fatalf(f string, a ...any) { panic(fmt.Errorf(f, a...)) }
func (c *Context) warnfAt(p src.XPos, f string, a ...any) {
	g := c.Backend.Typecheck.IR
	g.Diagnostics.Add(diagnostic.Warning, "", g.Base.Ctxt.OutermostPos(p), fmt.Sprintf(f, a...))
}

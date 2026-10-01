package arm64

import (
	"fmt"
	"go-backend-ssa/diagnostic"
	"go-backend-ssa/internal/cmd/obj"
	assembler "go-backend-ssa/internal/cmd/obj/arm64"
	"go-backend-ssa/internal/cmd/src"
	"go-backend-ssa/internal/cmd/sys"
	"go-backend-ssa/internal/ssa"
	"go-backend-ssa/internal/ssagen"
)

// Context binds the pinned ARM64 instruction generator to one compiler graph.
type Context struct {
	Backend          *ssagen.Context
	assembler        *assembler.Context
	LinkARM64        obj.LinkArch
	condBits         map[ssa.Op]assembler.SpecialOperand
	blockJump        map[ssa.BlockKind]struct{ asm, invasm obj.As }
	leJumps, gtJumps [2][2]ssagen.IndexJump
}

func NewContext(b *ssagen.Context) *Context {
	if b == nil || b.Typecheck == nil || b.Typecheck.IR == nil {
		panic("ARM64 emission requires a compiler graph")
	}
	if b.SSAConfig() != nil {
		panic("initialize ARM64 handlers before SSA configuration")
	}
	link := b.Typecheck.IR.Base.Ctxt
	if link == nil || link.Arch == nil || link.Arch.Arch == nil || link.Arch.Family != sys.ARM64 || b.Typecheck.IR.Types.PointerSize() != 8 {
		panic("ARM64 emission requires an ARM64 target")
	}
	c := &Context{Backend: b, LinkARM64: *link.Arch}
	c.initialize()
	c.assembler = assembler.NewContext(link, b.Typecheck.IR.Types.Experiment)
	c.LinkARM64 = c.assembler.Linkarm64
	c.Init(&b.Arch)
	link.Arch = b.Arch.LinkArch
	link.Arch.Init(link)
	return c
}
func (c *Context) fatalf(f string, a ...any) { panic(fmt.Errorf(f, a...)) }
func (c *Context) warnfAt(p src.XPos, f string, a ...any) {
	g := c.Backend.Typecheck.IR
	g.Diagnostics.Add(diagnostic.Warning, "", g.Base.Ctxt.OutermostPos(p), fmt.Sprintf(f, a...))
	g.Base.ReportWarning(p, f, a...)
}

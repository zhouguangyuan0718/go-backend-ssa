package noder

import (
	"go-backend-ssa/internal/base"
	"go-backend-ssa/internal/cmd/obj"
	"go-backend-ssa/internal/cmd/objabi"
	"go-backend-ssa/internal/cmd/src"
	"go-backend-ssa/internal/syntax"
)

// Context owns the formerly process-wide inputs used by the selected noder
// algorithms. The supplied Link and its position table belong to this instance.
type Context struct {
	Ctxt  *obj.Link
	Flag  base.CmdFlags
	paths objabi.PathContext
}

func (c *Context) absFile(dir, file, rewrites string) string {
	return c.paths.AbsFile(dir, file, rewrites)
}

type PosMap = posMap

// NewPosMap maps parser positions into the provided native Link position table.
// No cwd/GOROOT/environment lookup is performed. Do not copy an active map.
func NewPosMap(link *obj.Link, trimPath, goroot string) *PosMap {
	if link == nil {
		panic("nil native Link")
	}
	c := newContext()
	c.Ctxt = link
	c.Flag.TrimPath = trimPath
	c.paths.GOROOT = goroot
	return &posMap{compilation: c}
}
func (m *posMap) XPos(pos syntax.Pos) src.XPos             { return m.makeXPos(pos) }
func (m *posMap) TrimFilename(base *syntax.PosBase) string { return m.compilation.trimFilename(base) }

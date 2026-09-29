package objw

import (
	"fmt"
	"go-backend-ssa/internal/cmd/obj"
	"go-backend-ssa/internal/types"
)

// Context binds the native writer to an existing compiler Universe and Link.
// Callers serialize mutation of a Context and its symbols.
type Context struct {
	Types   *types.Universe
	Link    *obj.Link
	ptrSize int
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
	return &Context{Types: u, Link: link, ptrSize: u.PointerSize()}
}
func (c *Context) fatalf(format string, args ...any) { panic(fmt.Errorf(format, args...)) }

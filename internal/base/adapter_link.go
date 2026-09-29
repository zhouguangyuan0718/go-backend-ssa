package base

import "go-backend-ssa/internal/cmd/obj"

// LinkContext owns the native linker-symbol namespace inputs. The Link and its
// lookup maps stay intact; no second symbol representation is introduced.
type LinkContext struct {
	Flag            CmdFlags
	Ctxt            *obj.Link
	ReservedImports map[string]bool
}

func NewLinkContext(link *obj.Link) *LinkContext {
	if link == nil {
		panic("compiler symbols require a native Link")
	}
	c := newLinkContext()
	c.Ctxt = link
	return c
}

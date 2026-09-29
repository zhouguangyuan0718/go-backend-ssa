package base

import (
	"fmt"
	"go-backend-ssa/internal/cmd/obj"
	"go-backend-ssa/internal/go/bisect"
	"io"
)

// LinkContext owns the native linker-symbol namespace inputs. The Link and its
// lookup maps stay intact; no second symbol representation is introduced.
type LinkContext struct {
	hashDebug                                           *HashDebug
	ConvertHash, FmaHash, LoopVarHash, PGOHash          *HashDebug
	LiteralAllocHash, MergeLocalsHash, VariableMakeHash *HashDebug
	Timer                                               Timings
	Flag                                                CmdFlags
	Ctxt                                                *obj.Link
	ReservedImports                                     map[string]bool
}

func NewLinkContext(link *obj.Link) *LinkContext {
	if link == nil {
		panic("compiler symbols require a native Link")
	}
	c := newLinkContext()
	c.Ctxt = link
	return c
}

func (c *LinkContext) fatalf(format string, args ...any) { panic(fmt.Errorf(format, args...)) }

// log keeps native locking and both diagnostic formats. The writer supplied to
// NewHashDebug is the sole output destination; nil means discard. It never
// reads the process environment, opens files, or falls back to stdout.
func (d *HashDebug) log(varname string, hash uint64, text string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	file := d.logfile
	if file == nil {
		file = io.Discard
		d.logfile = file
	}
	fmt.Fprintf(file, "%s %s\n", text, bisect.Marker(hash))
	fmt.Fprintf(file, "%s triggered %s %s\n", varname, text, hashString(hash))
}

package base

import (
	"fmt"
	"go-backend-ssa/internal/cmd/obj"
	"go-backend-ssa/internal/cmd/objabi"
	"go-backend-ssa/internal/go/bisect"
	"io"
)

// LinkContext owns the native linker-symbol namespace inputs. The Link and its
// lookup maps stay intact; no second symbol representation is introduced.
type LinkContext struct {
	debugDefaults                                       DebugFlags
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

// DefaultDebugFlags executes the pinned compiler's initial debug assignments,
// before any command-line or environment overrides, into a detached value.
func DefaultDebugFlags() DebugFlags {
	c := newLinkContext()
	c.initializeDebugDefaults()
	return c.debugDefaults
}

// PrepareNativeFlags applies the pinned package/runtime normalization after
// explicit flags and debug settings have been supplied.
func (c *LinkContext) PrepareNativeFlags(debug *DebugFlags) {
	if c == nil || c.Ctxt == nil || debug == nil {
		panic("native flags require compiler state")
	}
	c.debugDefaults = *debug
	c.preparePackageFlags()
	c.normalizePackageFlags()
	*debug = c.debugDefaults
}
func (c *LinkContext) lookupPkgSpecial(path string) objabi.PkgSpecial {
	return objabi.NewContext().LookupPkgSpecial(path)
}

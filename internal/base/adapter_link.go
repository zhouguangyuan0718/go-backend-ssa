package base

import (
	"fmt"
	"go-backend-ssa/internal/cmd/obj"
	"go-backend-ssa/internal/cmd/objabi"
	"go-backend-ssa/internal/cmd/src"
	"go-backend-ssa/internal/go/bisect"
	"go-backend-ssa/internal/go/buildcfg"
	"io"
	"sync"
)

// LinkContext owns the native linker-symbol namespace inputs. The Link and its
// lookup maps stay intact; no second symbol representation is introduced.
type LinkContext struct {
	reportMu                                            sync.Mutex
	reportMessages                                      []errorMsg
	Experiment                                          buildcfg.ExperimentFlags
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
func (c *LinkContext) PrepareNativeFlags(debug *DebugFlags) (err error) {
	if c == nil || c.Ctxt == nil || debug == nil {
		panic("native flags require compiler state")
	}
	defer func() {
		if failure := recover(); failure != nil {
			if e, ok := failure.(debugSettingError); ok {
				err = e
			} else {
				panic(failure)
			}
		}
	}()
	c.debugDefaults = *debug
	c.prepareGeneralHashFlags()
	// Use the same native matcher validation without intercepting object-layer
	// failures. The setter owns the matcher's eventual symbol-selection state.
	if _, err = bisect.New(debug.FIPSHash); err != nil {
		return err
	}
	c.Ctxt.SetFIPSDebugHash(debug.FIPSHash)
	c.preparePackageFlags()
	c.prepareTransformHashFlags()
	for _, h := range []*HashDebug{c.hashDebug, c.LoopVarHash, c.ConvertHash, c.FmaHash, c.PGOHash, c.LiteralAllocHash, c.MergeLocalsHash, c.VariableMakeHash} {
		if h != nil {
			h.logfile = c.Ctxt.TraceWriter
		}
	}
	c.prepareLinkFlags()
	c.prepareInstrumentationFlags()
	c.prepareConcurrencyFlags()
	c.normalizePackageFlags()
	*debug = c.debugDefaults
	return nil
}

type debugSettingError struct{ message string }

func (e debugSettingError) Error() string { return e.message }
func (c *LinkContext) debugFlagFatalf(format string, args ...any) {
	panic(debugSettingError{fmt.Sprintf(format, args...)})
}

func (c *LinkContext) lookupPkgSpecial(path string) objabi.PkgSpecial {
	return objabi.NewContext().LookupPkgSpecial(path)
}

func (c *LinkContext) tracePrint(args ...any) (int, error) {
	w := c.Ctxt.TraceWriter
	if w == nil {
		w = io.Discard
	}
	return fmt.Fprint(w, args...)
}
func (c *LinkContext) traceWriter() io.Writer {
	if c.Ctxt.TraceWriter != nil {
		return c.Ctxt.TraceWriter
	}
	return io.Discard
}
func (c *LinkContext) tracePrintf(format string, args ...any) (int, error) {
	return fmt.Fprintf(c.traceWriter(), format, args...)
}
func (c *LinkContext) tracePrintln(args ...any) (int, error) {
	return fmt.Fprintln(c.traceWriter(), args...)
}

// Native command-line policy serializes debug compilation. Library callers can
// also report from independent backend workers, so the queue boundary owns a
// mutex without changing native message formatting, ordering, or deduplication.
func (c *LinkContext) ReportWarning(pos src.XPos, format string, args ...any) {
	c.reportMu.Lock()
	defer c.reportMu.Unlock()
	c.reportWarning(pos, format, args...)
}
func (c *LinkContext) FlushReports() { c.reportMu.Lock(); defer c.reportMu.Unlock(); c.flushReports() }

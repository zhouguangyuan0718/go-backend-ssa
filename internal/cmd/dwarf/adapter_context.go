package dwarf

import (
	"fmt"
	"go-backend-ssa/internal/go/buildcfg"
	"io"
)

// Emitter owns the native target-dependent abbreviation and encoding tables.
// Target inputs are fixed before any encoding or abbreviation finalization.
type Emitter struct {
	goos                   string
	experiment             buildcfg.ExperimentFlags
	logDwarf               bool
	sevenbits              [128]byte
	abbrevsFinalized       bool
	abbrevs, putvarAbbrevs []dwAbbrev
	TraceWriter            io.Writer
}

func NewEmitter(goos string, experiment buildcfg.ExperimentFlags) *Emitter {
	if goos == "" {
		panic("DWARF emitter requires an explicit target OS")
	}
	c := &Emitter{goos: goos, experiment: experiment}
	c.initialize()
	return c
}
func (c *Emitter) traceWriter() io.Writer {
	if c.TraceWriter != nil {
		return c.TraceWriter
	}
	return io.Discard
}
func (c *Emitter) tracePrintf(f string, args ...any) (int, error) {
	return fmt.Fprintf(c.traceWriter(), f, args...)
}
func (c *Emitter) tracePrint(args ...any) (int, error) { return fmt.Fprint(c.traceWriter(), args...) }
func (c *Emitter) tracePrintln(args ...any) (int, error) {
	return fmt.Fprintln(c.traceWriter(), args...)
}

package logopt

import (
	"go-backend-ssa/internal/cmd/src"
	"slices"
	"sync"
)

// Context owns the native log switch, event buffer, and backend recording lock.
// Set Format before concurrent recording; inspect snapshots after recording.
// A context is never copied after use. No process-global logger is installed.
type Context struct {
	Format     logFormat
	loggedOpts []*LoggedOpt
	mu         sync.Mutex
}

func NewContext() *Context { return newContext() }

// Snapshot retains original immutable event records in a fresh slice. The native
// event arguments may refer to IR and are borrowed; callers must keep them alive
// and avoid mutating them while reading. The buffer is not reset or drained.
func (c *Context) Snapshot() []*LoggedOpt {
	c.mu.Lock()
	defer c.mu.Unlock()
	return slices.Clone(c.loggedOpts)
}
func (o *LoggedOpt) Position() src.XPos     { return o.pos }
func (o *LoggedOpt) LastPosition() src.XPos { return o.lastPos }
func (o *LoggedOpt) Pass() string           { return o.compilerPass }
func (o *LoggedOpt) FunctionName() string   { return o.functionName }
func (o *LoggedOpt) What() string           { return o.what }
func (o *LoggedOpt) Arguments() []any       { return slices.Clone(o.target) }

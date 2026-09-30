// Package diagnostic provides compilation-owned diagnostic collection.
package diagnostic

import (
	"fmt"
	"go-backend-ssa/position"
	"sync"
)

type Severity uint8

const (
	Error Severity = iota
	Warning
)

// Diagnostic snapshots relative source coordinates so it does not retain a
// mutable position base or depend on later compilation activity.
type Diagnostic struct {
	Severity     Severity
	Code         string
	Filename     string
	Line, Column uint
	Message      string
}

func (d Diagnostic) String() string {
	where := d.Filename
	if d.Line != 0 {
		where += fmt.Sprintf(":%d", d.Line)
		if d.Column != 0 {
			where += fmt.Sprintf(":%d", d.Column)
		}
	}
	if where == "" {
		return d.Message
	}
	return where + ": " + d.Message
}

// Collector is owned by one compilation. Its zero value is ready for use.
// Backend workers may emit concurrently; separate collectors are independent.
// Do not copy a Collector after first use.
type Collector struct {
	mu      sync.Mutex
	entries []Diagnostic
}

func (c *Collector) Add(severity Severity, code string, pos position.Pos, message string) {
	entry := Diagnostic{severity, code, pos.RelFilename(), pos.RelLine(), pos.RelCol(), message}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = append(c.entries, entry)
}

// Diagnostics returns a detached snapshot in emission order.
func (c *Collector) Diagnostics() []Diagnostic {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]Diagnostic(nil), c.entries...)
}
func (c *Collector) HasErrors() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, d := range c.entries {
		if d.Severity == Error {
			return true
		}
	}
	return false
}

// RewriteLast refines an existing diagnostic only if it is still the last entry
// and matches the supplied snapshot. Earlier detached snapshots remain valid.
func (c *Collector) RewriteLast(expected Diagnostic, message string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	i := len(c.entries) - 1
	if i < 0 || c.entries[i] != expected {
		return false
	}
	c.entries[i].Message = message
	return true
}

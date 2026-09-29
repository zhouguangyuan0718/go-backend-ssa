// Package diagnostic provides compilation-owned diagnostic collection.
package diagnostic

import (
	"fmt"
	"go-backend-ssa/position"
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
// A compilation serializes its own writes; separate collectors are independent.
type Collector struct{ entries []Diagnostic }

func (c *Collector) Add(severity Severity, code string, pos position.Pos, message string) {
	c.entries = append(c.entries, Diagnostic{severity, code, pos.RelFilename(), pos.RelLine(), pos.RelCol(), message})
}

// Diagnostics returns a detached snapshot in emission order.
func (c *Collector) Diagnostics() []Diagnostic { return append([]Diagnostic(nil), c.entries...) }
func (c *Collector) HasErrors() bool {
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
	i := len(c.entries) - 1
	if i < 0 || c.entries[i] != expected {
		return false
	}
	c.entries[i].Message = message
	return true
}

// Package dwarfgen exposes native compiler DWARF variable, scope and inline emission.
package dwarfgen

import (
	compiler "go-backend-ssa/internal/dwarfgen"
	"go-backend-ssa/reflectdata"
	"go-backend-ssa/ssagen"
)

type Context = compiler.Context

// NewContext binds compiler callbacks and an instance-owned DWARF encoder before
// Progs.Flush. Fix target and experiment inputs before calling it.
func NewContext(backend *ssagen.Context, reflectData *reflectdata.Context) *Context {
	return compiler.NewContext(backend, reflectData)
}

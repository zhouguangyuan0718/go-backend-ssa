// Package gc exposes native function preparation, backend worker scheduling,
// compiler finalization and package archive output. The complete package fixed
// point and source driver are separate stages; callers share one compiler graph.
package gc

import (
	compiler "go-backend-ssa/internal/gc"
	"go-backend-ssa/objw"
)

type Context = compiler.Context

func NewContext(writer *objw.Context) *Context { return compiler.NewContext(writer) }

type ObjectConfig = compiler.ObjectConfig
type ObjectMode = compiler.ObjectMode

const (
	CompilerObject = compiler.CompilerObject
	LinkerObject   = compiler.LinkerObject
	CombinedObject = compiler.CombinedObject
)

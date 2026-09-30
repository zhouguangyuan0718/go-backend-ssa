// Package gc exposes native package frontend passes, backend worker scheduling,
// declaration fixed points, data finalization and package archive output.
// The source driver is a separate stage; callers share one compiler graph.
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

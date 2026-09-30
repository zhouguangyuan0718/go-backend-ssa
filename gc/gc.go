// Package gc exposes native compiler finalization and package archive output.
// It is not a
// complete compiler driver; each phase uses the existing compilation graph.
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

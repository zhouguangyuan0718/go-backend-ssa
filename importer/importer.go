// Package importer reads the original compiler's finalized Unified export data
// into an instance-owned types2 graph, preserving native lazy type loading.
package importer

import (
	compiler "go-backend-ssa/internal/importer"
	"go-backend-ssa/types2"
)

type Context = compiler.ImportContext

func NewContext(checker *types2.Context) *Context { return compiler.NewContext(checker) }

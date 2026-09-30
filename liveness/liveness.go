// Package liveness exposes native local-merging services for one compilation.
package liveness

import (
	compiler "go-backend-ssa/internal/liveness"
	"go-backend-ssa/ssa"
)

type Context = compiler.Context
type MergeLocalsState = compiler.MergeLocalsState

func NewContext(s *ssa.Context) *Context { return compiler.NewContext(s) }

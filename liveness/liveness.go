// Package liveness exposes native stack maps, argument liveness and local merging.
package liveness

import (
	compiler "go-backend-ssa/internal/liveness"
	"go-backend-ssa/ssa"
)

type Context = compiler.Context
type Map = compiler.Map
type Liveness = compiler.Liveness
type MergeLocalsState = compiler.MergeLocalsState

func NewContext(s *ssa.Context) *Context { return compiler.NewContext(s) }

// Package compare exposes native comparison construction and cost queries.
// The caller must evaluate returned conditions in order and short-circuit them.
package compare

import (
	compiler "go-backend-ssa/internal/compare"
	"go-backend-ssa/typecheck"
	"go-backend-ssa/types"
)

type Context = compiler.Context

func NewContext(tc *typecheck.Context) *Context    { return compiler.NewContext(tc) }
func IsRegularMemory(t *types.Type) bool           { return compiler.IsRegularMemory(t) }
func Memrun(t *types.Type, start int) (int64, int) { return compiler.Memrun(t, start) }
func EqCanPanic(t *types.Type) bool                { return compiler.EqCanPanic(t) }
func EqStructCost(t *types.Type) int64             { return compiler.EqStructCost(t) }

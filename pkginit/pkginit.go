// Package pkginit exposes native package initialization records and ASan global
// registration. Run MakeTask once after loop capture rewriting, before ABI
// wrappers and escape analysis, using the same noder instance as the frontend.
package pkginit

import (
	compiler "go-backend-ssa/internal/pkginit"
	"go-backend-ssa/noder"
)

type Context = compiler.Context

func NewContext(reader *noder.Context) *Context { return compiler.NewContext(reader) }
func GetRedzoneSizeForGlobal(size int64) int64  { return compiler.GetRedzoneSizeForGlobal(size) }

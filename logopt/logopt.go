// Package logopt exposes the native optimizer event collector. Each compilation
// owns its recording state; CLI destination parsing and LSP file emission are
// not exposed by this entry point.
package logopt

import (
	compiler "go-backend-ssa/internal/logopt"
	"go-backend-ssa/position"
)

type Context = compiler.Context
type LoggedOpt = compiler.LoggedOpt

const None = compiler.None
const Json0 = compiler.Json0

func NewContext() *Context { return compiler.NewContext() }
func NewLoggedOpt(pos, lastPos position.XPos, what, pass, funcName string, args ...any) *LoggedOpt {
	return compiler.NewLoggedOpt(pos, lastPos, what, pass, funcName, args...)
}

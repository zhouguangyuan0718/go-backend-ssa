// Package slice exposes the native Go compiler optimization of slice backing
// stores. Run Funcs after escape analysis and before walk/SSA construction.
package slice

import (
	"go-backend-ssa/escape"
	compiler "go-backend-ssa/internal/slice"
	"go-backend-ssa/reflectdata"
)

type Context = compiler.Context

func NewContext(esc *escape.Context, rd *reflectdata.Context) *Context {
	return compiler.NewContext(esc, rd)
}

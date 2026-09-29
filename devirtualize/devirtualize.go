// Package devirtualize exposes Go's native static and profile-guided call
// devirtualization. A zero State binds to a Context on its first StaticCall.
// State.InlinedCall must use that same graph after an inline replacement.
package devirtualize

import (
	"go-backend-ssa/inline"
	compiler "go-backend-ssa/internal/devirtualize"
)

type Context = compiler.Context
type State = compiler.State
type CallStat = compiler.CallStat

func NewContext(inl *inline.Context) *Context { return compiler.NewContext(inl) }

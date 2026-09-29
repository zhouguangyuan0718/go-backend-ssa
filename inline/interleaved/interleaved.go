// Package interleaved exposes the native fixed-point devirtualization and
// inlining drivers. noder.NewContext binds its Interleaved field to the reader.
// Package calls operate on Typecheck.Target and mutate its functions in place.
package interleaved

import (
	"go-backend-ssa/inline"
	compiler "go-backend-ssa/internal/inline/interleaved"
)

type Context = compiler.Context

func NewContext(inl *inline.Context) *Context { return compiler.NewContext(inl) }

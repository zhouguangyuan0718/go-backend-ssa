// Package inline exposes the native compiler's inlinability and call-site
// expansion algorithms. noder.NewContext binds InlineCall to its native reader.
// TryInlineCall runs inside IR.WithFunc(caller, ...); install its non-nil result
// in the caller's graph. Package inline/interleaved exposes the native drivers.
package inline

import (
	"go-backend-ssa/inline/inlheur"
	compiler "go-backend-ssa/internal/inline"
	"go-backend-ssa/logopt"
	"go-backend-ssa/typecheck"
)

type Context = compiler.Context

func NewContext(tc *typecheck.Context, heur *inlheur.Context, log *logopt.Context) *Context {
	return compiler.NewContext(tc, heur, log)
}

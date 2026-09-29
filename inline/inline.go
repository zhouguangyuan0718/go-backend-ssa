// Package inline exposes the native compiler's inlinability analysis. Call-site
// expansion and the complete optimization driver are not yet integrated.
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

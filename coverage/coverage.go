// Package coverage exposes the native Go compiler coverage metadata pass.
package coverage

import (
	"go-backend-ssa/internal/cmd/cov/covcmd"
	compiler "go-backend-ssa/internal/coverage"
	"go-backend-ssa/typecheck"
)

type Context = compiler.Context

// Config is the native coverage configuration produced by cmd/cover. Set it
// on the compilation flag Cfg.CoverageInfo before calling Fixup.
type Config = covcmd.CoverFixupConfig

func NewContext(tc *typecheck.Context) *Context { return compiler.NewContext(tc) }

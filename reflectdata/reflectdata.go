// Package reflectdata exposes the native compiler's runtime type symbols and
// descriptors, ITabs, IR references, GC bitmap data and equality functions.
// Initialize the attached typecheck runtime declarations before generating
// descriptors, ITabs or equality functions. Method wrapper bodies are created
// by the unified frontend; this package emits their native symbol references.
package reflectdata

import (
	compiler "go-backend-ssa/internal/reflectdata"
	"go-backend-ssa/rttype"
	"go-backend-ssa/staticdata"
	"go-backend-ssa/typecheck"
)

type Context = compiler.Context

// NewContext binds typechecking, initialized runtime layouts and static data from one
// compilation. Serialize phase operations. GCSym supports concurrent registration
// with complete, unchanged type layouts; join callers before WriteGCSymbols.
func NewContext(tc *typecheck.Context, rt *rttype.Context, sd *staticdata.Context) *Context {
	return compiler.NewContext(tc, rt, sd)
}

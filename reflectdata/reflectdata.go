// Package reflectdata exposes the native compiler's runtime type symbols and
// IR references. Descriptor and ITab emission are not yet ported.
package reflectdata

import (
	compiler "go-backend-ssa/internal/reflectdata"
	"go-backend-ssa/rttype"
	"go-backend-ssa/typecheck"
)

type Context = compiler.Context

// NewContext binds initialized typechecking and runtime layouts from one
// compilation. Calls and mutation of the attached graphs must be serialized.
func NewContext(tc *typecheck.Context, rt *rttype.Context) *Context {
	return compiler.NewContext(tc, rt)
}

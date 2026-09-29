// Package reflectdata exposes the native compiler's runtime type symbols and
// IR references and GC bitmap data. Full descriptor and ITab emission are not yet ported.
package reflectdata

import (
	compiler "go-backend-ssa/internal/reflectdata"
	"go-backend-ssa/rttype"
	"go-backend-ssa/typecheck"
)

type Context = compiler.Context

// NewContext binds initialized typechecking and runtime layouts from one
// compilation. Serialize phase operations. GCSym supports concurrent registration
// with complete, unchanged type layouts; join callers before WriteGCSymbols.
func NewContext(tc *typecheck.Context, rt *rttype.Context) *Context {
	return compiler.NewContext(tc, rt)
}

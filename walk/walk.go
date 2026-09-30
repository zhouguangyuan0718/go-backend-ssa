// Package walk exposes the native Go compiler's IR ordering and lowering pass.
// Run it after escape analysis and slice optimization. Walk mutates the function
// and leaves IR.CurFunc set as in the upstream driver contract. Independent
// compiler graphs can run concurrently; functions within one graph run serially.
// As in gc.prepareFunc, the driver must initialize each function's LSym and size
// its signature before Walk, then walk its closures and clear CurFunc afterward.
package walk

import (
	"go-backend-ssa/escape"
	compiler "go-backend-ssa/internal/walk"
	"go-backend-ssa/noder"
)

type Context = compiler.Context

func NewContext(nd *noder.Context, esc *escape.Context) *Context {
	return compiler.NewContext(nd, esc)
}

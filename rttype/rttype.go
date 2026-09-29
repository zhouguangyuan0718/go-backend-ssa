// Package rttype exposes the native compiler's runtime type layouts and typed
// static-data cursors. These are target layouts, independent of the host width.
package rttype

import (
	compiler "go-backend-ssa/internal/rttype"
	"go-backend-ssa/objw"
)

type Context = compiler.Context
type Cursor = compiler.Cursor
type ArrayCursor = compiler.ArrayCursor

// NewContext uses the writer's initialized compiler Universe. Call Init once
// before accessing the runtime layout fields; serialize use with the writer.
func NewContext(writer *objw.Context) *Context { return compiler.NewContext(writer) }

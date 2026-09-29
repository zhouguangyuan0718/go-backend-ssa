package gcbridge

import (
	"go-backend-ssa/internal/cmd/obj"
	"go-backend-ssa/internal/types"
)

// These aliases expose the same objects, not parallel host-model copies.
// Compiler algorithms and native code generation are still being integrated.
type Universe = types.Universe
type Type = types.Type
type Pkg = types.Pkg
type Sym = types.Sym
type Object = types.Object
type LinkSymbol = obj.LSym

type Field = types.Field
type Struct = types.Struct

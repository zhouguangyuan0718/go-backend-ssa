// Package typebits marks native compiler types' GC pointer slots in a bitmap.
// The target width comes from the type's Universe. Type layouts must be ready
// before concurrent use; Set mutates the caller's bitmap.
package typebits

import (
	"go-backend-ssa/bitvec"
	compiler "go-backend-ssa/internal/typebits"
	"go-backend-ssa/types"
)

func Set(t *types.Type, off int64, bv bitvec.BitVec)        { compiler.Set(t, off, bv) }
func SetNoCheck(t *types.Type, off int64, bv bitvec.BitVec) { compiler.SetNoCheck(t, off, bv) }

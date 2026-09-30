// Package objw exposes the native compiler's object data and relocation writer.
package objw

import (
	"go-backend-ssa/gcbridge"
	"go-backend-ssa/internal/bitvec"
	"go-backend-ssa/internal/cmd/obj"
	compiler "go-backend-ssa/internal/objw"
	"go-backend-ssa/types"
)

type Context = compiler.Context
type Progs = compiler.Progs
type StackMapIndex = compiler.StackMapIndex

const StackMapDontCare = compiler.StackMapDontCare

type BitVec = bitvec.BitVec

const (
	DUPOK  = obj.DUPOK
	RODATA = obj.RODATA
	NOPTR  = obj.NOPTR
	LOCAL  = obj.LOCAL
	TLSBSS = obj.TLSBSS
)

// NewContext uses the existing Link's architecture, byte order and position
// table. goos is explicit and cannot change after binding. The caller supplies
// Link.DiagFunc for native recoverable diagnostics and serializes symbol writes.
func NewContext(u *types.Universe, link *gcbridge.Link, goos string) *Context {
	return compiler.NewContext(u, link, goos)
}

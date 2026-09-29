// Package bitvec exposes the native compiler's bitmap operations.
package bitvec

import compiler "go-backend-ssa/internal/bitvec"

type BitVec = compiler.BitVec
type Bulk = compiler.Bulk

func New(n int32) BitVec             { return compiler.New(n) }
func NewBulk(nbit, count int32) Bulk { return compiler.NewBulk(nbit, count) }

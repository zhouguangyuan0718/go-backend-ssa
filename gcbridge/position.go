// Package gcbridge is the version-bound native compiler integration surface.
// Currently only source position identity and conversion are implemented.
package gcbridge

import "go-backend-ssa/internal/cmd/src"

type Pos = src.Pos
type XPos = src.XPos
type PosBase = src.PosBase
type PosTable = src.PosTable

// Encode uses the caller's compilation-owned table. The base must not have been
// registered in another table. As in upstream, violating that rule panics.
func Encode(table *PosTable, pos Pos) XPos { return table.XPos(pos) }

// Decode requires the table that originally encoded pos; compressed indices do
// not identify their table. No objects are copied into a second representation.
func Decode(table *PosTable, pos XPos) Pos { return table.Pos(pos) }

// Package position exposes the ported compiler's source position objects.
// Bases and tables belong to one compilation. Mutation, including encoding into
// a table, requires exclusive access; separate compilations use separate bases.
package position

import "go-backend-ssa/internal/cmd/src"

// Aliases preserve identity with the internal compiler and native bridge.
type Pos = src.Pos
type Base = src.PosBase
type XPos = src.XPos
type Table = src.PosTable
type Xlogue = src.PosXlogue

const (
	DefaultStmt   = src.PosDefaultStmt
	IsStmt        = src.PosIsStmt
	NotStmt       = src.PosNotStmt
	DefaultLogue  = src.PosDefaultLogue
	PrologueEnd   = src.PosPrologueEnd
	EpilogueBegin = src.PosEpilogueBegin
)

func Make(base *Base, line, col uint) Pos { return src.MakePos(base, line, col) }
func NewFile(filename, absoluteFilename string) *Base {
	return src.NewFileBase(filename, absoluteFilename)
}
func NewLine(pos Pos, filename, absoluteFilename string, line, col uint) *Base {
	return src.NewLinePragmaBase(pos, filename, absoluteFilename, line, col)
}
func NewInlining(base *Base, index int) *Base { return src.NewInliningBase(base, index) }

// Unknown positions are zero values: Pos{} and XPos{}. There is no global sentinel.
// XPos is meaningful only with its originating Table; it does not carry an owner
// tag. Never resolve it through another compilation's table, even if indices match.

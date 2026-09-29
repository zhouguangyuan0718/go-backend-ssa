// Package syntax exposes Go 1.27's compiler parser and its native syntax tree.
// Parse owns its scanner/branch state and uses only explicit input and callbacks.
// Returned trees and position bases remain caller-owned; parsing does not type
// check, resolve imports, or produce compiler IR/SSA.
package syntax

import (
	compiler "go-backend-ssa/internal/syntax"
	"io"
	"regexp"
)

// Parse retains the upstream error contract: without errh, stop at the first
// syntax error and return a nil tree; with errh, report errors and return the
// partial tree when possible. Readers and callbacks must remain valid for this
// call. Independent or nested calls share no mutable parser state.
func Parse(base *PosBase, src io.Reader, errh ErrorHandler, pragh PragmaHandler, mode Mode) (*File, error) {
	// Upstream's branch checker calls errh directly, and does not update the
	// parser's first-error field. Supply a handler even when the caller omits
	// one, and include branch errors in the public Parse result.
	var first error
	handler := func(err error) {
		if first == nil {
			first = err
		}
		if errh == nil {
			panic(err)
		}
		errh(err)
	}
	file, err := compiler.Parse(base, src, handler, pragh, mode)
	if err == nil {
		err = first
	}
	return file, err
}
func MakePos(base *PosBase, line, col uint) Pos { return compiler.MakePos(base, line, col) }
func NewFileBase(filename string) *PosBase      { return compiler.NewFileBase(filename) }
func NewTrimmedFileBase(filename string, trimmed bool) *PosBase {
	return compiler.NewTrimmedFileBase(filename, trimmed)
}
func NewLineBase(pos Pos, filename string, trimmed bool, line, col uint) *PosBase {
	return compiler.NewLineBase(pos, filename, trimmed, line, col)
}
func NewName(pos Pos, value string) *Name                { return compiler.NewName(pos, value) }
func Unparen(x Expr) Expr                                { return compiler.Unparen(x) }
func UnpackListExpr(x Expr) []Expr                       { return compiler.UnpackListExpr(x) }
func StartPos(n Node) Pos                                { return compiler.StartPos(n) }
func EndPos(n Node) Pos                                  { return compiler.EndPos(n) }
func Inspect(root Node, f func(Node) bool)               { compiler.Inspect(root, f) }
func Walk(root Node, v Visitor)                          { compiler.Walk(root, v) }
func Fprint(w io.Writer, n Node, form Form) (int, error) { return compiler.Fprint(w, n, form) }
func String(n Node) string                               { return compiler.String(n) }
func Fdump(w io.Writer, n Node) error                    { return compiler.Fdump(w, n) }
func CommentsDo(src io.Reader, handler func(line, col uint, text string)) {
	compiler.CommentsDo(src, handler)
}
func CommentMap(src io.Reader, rx *regexp.Regexp) map[uint][]Error {
	return compiler.CommentMap(src, rx)
}

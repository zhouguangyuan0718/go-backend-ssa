// Package noder exposes migrated pieces of the native AST-to-compiler-IR
// pipeline. Package IR construction is still being integrated.
package noder

import (
	"go-backend-ssa/gcbridge"
	compiler "go-backend-ssa/internal/noder"
)

// PosMap is the native cached syntax.Pos-to-XPos mapper. Each instance uses the
// supplied Link's position table. Do not copy or concurrently mutate a map or
// its Link; independent maps and Links may be used concurrently.
type PosMap = compiler.PosMap

// NewPosMap takes explicit path inputs: Link.Pathname is the compilation's
// working directory, trimPath is the native -trimpath rule string, and goroot
// is the source GOROOT to normalize. No process environment is consulted.
func NewPosMap(link *gcbridge.Link, trimPath, goroot string) *PosMap {
	return compiler.NewPosMap(link, trimPath, goroot)
}

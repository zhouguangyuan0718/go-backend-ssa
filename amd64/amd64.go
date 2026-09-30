// Package amd64 exposes the native AMD64 instruction generator and assembler.
package amd64

import (
	compiler "go-backend-ssa/internal/amd64"
	"go-backend-ssa/ssagen"
)

type Context = compiler.Context

// NewContext installs and initializes native compiler/assembler handlers before
// backend.InitConfig. Full object output is a later operation.
func NewContext(backend *ssagen.Context) *Context { return compiler.NewContext(backend) }

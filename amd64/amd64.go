// Package amd64 exposes the native AMD64 instruction generator.
package amd64

import (
	compiler "go-backend-ssa/internal/amd64"
	"go-backend-ssa/ssagen"
)

type Context = compiler.Context

// NewContext installs the native handlers before backend.InitConfig.
func NewContext(backend *ssagen.Context) *Context { return compiler.NewContext(backend) }

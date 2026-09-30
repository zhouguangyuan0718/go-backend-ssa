// Package arm64 exposes the native ARM64 instruction generator and assembler.
package arm64

import (
	compiler "go-backend-ssa/internal/arm64"
	"go-backend-ssa/ssagen"
)

type Context = compiler.Context

// NewContext installs and initializes the native target before backend.InitConfig.
func NewContext(backend *ssagen.Context) *Context { return compiler.NewContext(backend) }

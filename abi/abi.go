// Package abi exposes the native compiler's parameter/register layout analysis.
// A Context owns synthetic ABI types for one IR graph; use independent contexts
// for independent compilations, and serialize layout work on the same graph.
package abi

import (
	compiler "go-backend-ssa/internal/abi"
	"go-backend-ssa/ir"
)

type Context = compiler.Context
type ABIConfig = compiler.ABIConfig
type ABIParamResultInfo = compiler.ABIParamResultInfo
type ABIParamAssignment = compiler.ABIParamAssignment
type RegIndex = compiler.RegIndex

func NewContext(graph *ir.Context) *Context { return compiler.NewContext(graph) }

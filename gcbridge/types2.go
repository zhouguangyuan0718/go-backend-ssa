package gcbridge

import "go-backend-ssa/internal/types2"

// Source-checker objects are the native types2 objects, distinct from compiler
// IR's types.Type. Noder integration must translate them through upstream code.
type CheckerContext = types2.Context
type CheckerConfig = types2.Config
type CheckedPackage = types2.Package
type SourceType = types2.Type
type SourceObject = types2.Object

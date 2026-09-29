package gcbridge

import "go-backend-ssa/internal/syntax"

// Syntax aliases share the parser's tree and source bases with a compiler host.
// They do not translate this tree into compiler IR or lower it to SSA.
type SyntaxFile = syntax.File
type SyntaxNode = syntax.Node
type SyntaxPos = syntax.Pos
type SyntaxPosBase = syntax.PosBase

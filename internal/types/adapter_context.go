// Library-owned string interning context; upstream algorithms live in pkg.go.
package types

import "sync"

// Context must not be copied after first use. Separate contexts share no table.
// Its method algorithm and constructor are ported from the pinned compiler.
type Context struct {
	mu      sync.Mutex
	strings map[string]string
}

func NewContext() *Context { return newContext() }

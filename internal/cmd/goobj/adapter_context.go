package goobj

import "go-backend-ssa/internal/go/buildcfg"

// Context owns the native builtin symbol table and ABI interpretation.
type Context struct {
	experiment buildcfg.ExperimentFlags
	builtinMap map[string]int
	builtins   [299]struct {
		name string
		abi  int
	}
}

func NewContext(regabiWrappers bool) *Context {
	c := &Context{}
	c.experiment.RegabiWrappers = regabiWrappers
	c.initialize()
	return c
}

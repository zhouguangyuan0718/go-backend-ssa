package objabi

import "go-backend-ssa/internal/go/buildcfg"

// Context supplies the native path setting and owns the lazy package-property
// cache. Package classifications still come from the upstream tables.
type Context struct {
	ObjectConfig                                                        *buildcfg.Context
	Experiment                                                          buildcfg.ExperimentFlags
	GOROOT                                                              string
	runtimePkgs, extraNoInstrumentPkgs, noRaceFuncPkgs, allowAsmABIPkgs []string
	pkgSpecialsOnce                                                     func() map[string]PkgSpecial
}

func NewContext() *Context { return newContext() }

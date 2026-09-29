package objabi

// Context supplies the native path setting and owns the lazy package-property
// cache. Package classifications still come from the upstream tables.
type Context struct {
	GOROOT                                                              string
	runtimePkgs, extraNoInstrumentPkgs, noRaceFuncPkgs, allowAsmABIPkgs []string
	pkgSpecialsOnce                                                     func() map[string]PkgSpecial
}

func NewContext() *Context { return newContext() }

package sys

// Context owns the native architecture descriptors and preserves their pointer
// identities within one compilation setup. Independent sets share no descriptors.
type Context struct {
	Arch386, ArchAMD64, ArchARM, ArchARM64, ArchLoong64      *Arch
	ArchMIPS, ArchMIPSLE, ArchMIPS64, ArchMIPS64LE           *Arch
	ArchPPC64, ArchPPC64LE, ArchRISCV64, ArchS390X, ArchWasm *Arch
	Archs                                                    [14]*Arch
}

// ContextOfArch is nil for a caller-built descriptor. Such a descriptor does
// not have the identity of a canonical architecture, just as in the native code.
func ContextOfArch(a *Arch) *Context {
	if a == nil {
		return nil
	}
	return a.compilation
}

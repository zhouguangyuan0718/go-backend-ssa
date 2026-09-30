package x86

import "go-backend-ssa/internal/cmd/obj"

// Context installs the native x86 register/opcode formatters on one Link.
type Context struct {
	AMD64DWARFRegisters map[int16]int16
	isAndroid           bool
	Link                *obj.Link
	Anames              []string
	opSuffixTable       [15]string
	evexSuffixMap       [255]evexSuffix
}

func NewContext(link *obj.Link) *Context {
	if link == nil || link.Arch == nil || link.Arch.Arch == nil {
		panic("x86 formatting requires a target")
	}
	c := &Context{Link: link}
	c.initialize()
	return c
}

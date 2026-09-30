package obj

import (
	"fmt"
	"go-backend-ssa/internal/cmd/dwarf"
	"io"
)

// dataState holds host inputs absent from the original Link. TargetOS must be
// supplied before writing symbols; the architecture remains Link.Arch.
type dataState struct {
	inlHashLen      int
	DWARF           *dwarf.Emitter
	textAttrStrings [19]struct {
		bit Attribute
		s   string
	}
	opSuffixSpace []opSuffixSet
	regSpace      []regSet
	regListSpace  []regListSet
	spcSpace      []spcSet
	aSpace        []opSet
	TargetOS      string
	TraceWriter   io.Writer
}

func dataLink(link *Link) *Link {
	if link == nil || link.Arch == nil || link.Arch.Arch == nil || link.TargetOS == "" {
		panic("object data requires an explicit target OS and architecture")
	}
	return link
}

// Grow needs only a stateless panic boundary for its native log.Fatalf path.
// It must not acquire a compilation owner or change its native signature.
func statelessDataLink(*LSym) *Link                     { return &Link{} }
func (link *Link) fatalLog(args ...any)                 { panic(fmt.Sprint(args...)) }
func (link *Link) fatalLogf(format string, args ...any) { panic(fmt.Sprintf(format, args...)) }
func (link *Link) traceWriter() io.Writer {
	if link.TraceWriter != nil {
		return link.TraceWriter
	}
	return io.Discard
}

func progLink(p *Prog) *Link {
	if p == nil || p.Ctxt == nil {
		return &Link{}
	}
	return p.Ctxt
}

func attributeLink(Attribute) *Link { c := &Link{}; c.initializeData(); return c }

func pcIterLink(*PCIter) *Link { return &Link{} }
func (link *Link) tracePrintf(f string, args ...any) (int, error) {
	return fmt.Fprintf(link.traceWriter(), f, args...)
}

func inlineHashLink(string) *Link { c := &Link{}; c.initializeData(); return c }

func (link *Link) checkFlushplist(ctxt *Link, plist *Plist, newprog ProgAlloc) {
	if ctxt != link || plist == nil || link.Arch.Preprocess == nil || link.Arch.Assemble == nil || link.DWARF == nil {
		panic("flush requires an owned program list and configured assembler/DWARF encoder")
	}
	seen := make(map[*Prog]bool)
	for p := plist.Firstpc; p != nil; p = p.Link {
		if seen[p] || p.Ctxt != link {
			panic("flush requires an owned acyclic instruction list")
		}
		seen[p] = true
	}
}

// EnsureDwarfFixups initializes native fixups before inlining, preserving an
// existing table from the same compilation.
func (link *Link) EnsureDwarfFixups() {
	if link.DwFixups != nil && link.DwFixups.ctxt != link {
		panic("DWARF fixups belong to another compilation")
	}
	if link.DwFixups == nil {
		link.DwFixups = NewDwarfFixupTable(link)
	}
}

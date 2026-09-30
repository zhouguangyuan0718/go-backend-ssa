package obj

import (
	"fmt"
	"go-backend-ssa/internal/cmd/bio"
	"go-backend-ssa/internal/cmd/dwarf"
	"go-backend-ssa/internal/cmd/goobj"
	"io"
)

// dataState holds host inputs absent from the original Link. TargetOS must be
// supplied before writing symbols; the architecture remains Link.Arch.
type dataState struct {
	objectNumbered, objectWritten bool
	ObjectBuiltins                *goobj.Context
	OpenObjectInput               func(string) (io.ReadCloser, error)
	inlHashLen                    int
	DWARF                         *dwarf.Emitter
	textAttrStrings               [19]struct {
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

func objectWriterLink(w *writer) *Link {
	if w == nil {
		panic("object writer is nil")
	}
	return dataLink(w.ctxt)
}
func (link *Link) openObjectInput(name string) (io.ReadCloser, error) {
	if link.OpenObjectInput == nil {
		return nil, fmt.Errorf("object input reader is not configured: %s", name)
	}
	return link.OpenObjectInput(name)
}
func (link *Link) PrepareObject(regabiWrappers bool) {
	if link.ObjectBuiltins != nil {
		panic("object tables already configured")
	}
	link.ObjectBuiltins = goobj.NewContext(regabiWrappers)
}

func (link *Link) checkNumberSyms() {
	dataLink(link)
	if link.ObjectBuiltins == nil || link.objectNumbered || link.Errors != 0 {
		panic("symbol numbering requires configured, unconsumed object state without errors")
	}
	link.objectNumbered = true
}
func (link *Link) checkWriteObjFile(ctxt *Link, b *bio.Writer) {
	if ctxt != link || !link.objectNumbered || link.objectWritten || link.Errors != 0 {
		panic("object serialization requires numbered, unconsumed state without errors")
	}
	b.Validate()
	link.objectWritten = true
}

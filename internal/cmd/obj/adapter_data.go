package obj

import (
	"fmt"
	"io"
)

// dataState holds host inputs absent from the original Link. TargetOS must be
// supplied before writing symbols; the architecture remains Link.Arch.
type dataState struct {
	TargetOS    string
	TraceWriter io.Writer
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

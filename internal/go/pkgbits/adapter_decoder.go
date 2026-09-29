package pkgbits

import (
	"fmt"
	"io"
)

// TraceWriter receives the upstream desynchronization diagnostic. Nil discards
// output. The decoder never terminates the process or writes ambient stdout.
type decoderIO struct{ TraceWriter io.Writer }

type discardTrace struct{}

func (discardTrace) Write(p []byte) (int, error) { return len(p), nil }
func (pr *PkgDecoder) tracePrintf(format string, args ...any) (int, error) {
	var out io.Writer = discardTrace{}
	if pr.TraceWriter != nil {
		out = pr.TraceWriter
	}
	return fmt.Fprintf(out, format, args...)
}

// DecodeAbort is the recoverable equivalent of the native decoder's os.Exit.
// Other malformed-data failures retain upstream panic values.
type DecodeAbort struct{ Status int }

func (e DecodeAbort) Error() string {
	return fmt.Sprintf("pkgbits: decoder aborted (status %d)", e.Status)
}
func (pr *PkgDecoder) abort(status int) { panic(DecodeAbort{Status: status}) }

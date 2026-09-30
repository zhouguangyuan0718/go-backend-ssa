package bio

import (
	"bufio"
	"fmt"
	"io"
)

type Context struct{}

// OutputError preserves a native fatal stream diagnostic without exiting the
// host process, and retains the underlying I/O error for errors.Is/As.
type OutputError struct {
	Message string
	Cause   error
}

func (e *OutputError) Error() string { return e.Message }
func (e *OutputError) Unwrap() error { return e.Cause }
func writerContext(w *Writer) *Context {
	if w == nil || w.f == nil || w.Writer == nil {
		panic("object output requires an explicit stream")
	}
	return &Context{}
}
func (*Context) fatalLogf(f string, a ...any) {
	e := &OutputError{Message: fmt.Sprintf(f, a...)}
	if len(a) == 1 {
		e.Cause, _ = a[0].(error)
	}
	panic(e)
}

// NewWriter binds the native buffered writer to an explicit seekable output.
// The caller owns and closes the stream.
func NewWriter(out io.WriteSeeker) *Writer {
	if out == nil {
		panic("object output is nil")
	}
	return &Writer{f: borrowedOutput{out}, Writer: bufio.NewWriter(out)}
}
func (w *Writer) Validate() { _ = writerContext(w) }

// A borrowed stream remains owned by the caller after native Writer.Close.
type borrowedOutput struct{ io.WriteSeeker }

func (borrowedOutput) Close() error { return nil }

func readerContext(r *Reader) *Context {
	if r == nil || r.f == nil || r.Reader == nil {
		panic("object input requires an explicit stream")
	}
	return &Context{}
}

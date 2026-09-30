package bio

import (
	"bytes"
	"errors"
	"io"
	"sync"
	"testing"
)

type testInput struct {
	*bytes.Reader
	closed            int
	seekErr, closeErr error
}

func (s *testInput) Close() error { s.closed++; return s.closeErr }
func (s *testInput) Seek(n int64, w int) (int64, error) {
	if s.seekErr != nil {
		return 0, s.seekErr
	}
	return s.Reader.Seek(n, w)
}
func TestNativeBufferedInput(t *testing.T) {
	var wg sync.WaitGroup
	for seed := range 8 {
		wg.Go(func() {
			data := bytes.Repeat([]byte{byte(seed)}, 10000)
			s := &testInput{Reader: bytes.NewReader(data)}
			r := NewReader(s)
			if b, e := r.ReadByte(); b != byte(seed) || e != nil {
				t.Error(b, e)
			}
			if r.Buffered() == 0 || r.Offset() != 1 {
				t.Error("read ahead offset", r.Buffered(), r.Offset())
			}
			if n := r.MustSeek(7, io.SeekCurrent); n != 8 {
				t.Error(n)
			}
			if r.Offset() != 8 || r.Buffered() != 0 {
				t.Error("seek did not reset buffer")
			}
			b := make([]byte, 9)
			if n, e := io.ReadFull(r, b); n != 9 || e != nil || !bytes.Equal(b, data[:9]) {
				t.Error(n, e, b)
			}
			if n := r.MustSeek(-3, io.SeekEnd); n != 9997 {
				t.Error(n)
			}
			if b, e := io.ReadAll(r); len(b) != 3 || e != nil {
				t.Error(len(b), e)
			}
			if n, e := r.File().ReadAt(b[:1], 3); n != 1 || e != nil || r.Offset() != 10000 {
				t.Error(n, e, r.Offset())
			}
			s.closeErr = errors.New("close")
			if r.File() != s || r.Close() != s.closeErr || s.closed != 1 {
				t.Error("stream identity/close", s.closed)
			}
		})
	}
	wg.Wait()
}
func TestNativeInputSeekDiagnostic(t *testing.T) {
	for _, offset := range []bool{false, true} {
		func() {
			failure := errors.New("seek failure")
			s := &testInput{Reader: bytes.NewReader([]byte("abcdef"))}
			r := NewReader(s)
			r.ReadByte()
			s.seekErr = failure
			defer func() {
				p := recover()
				e, ok := p.(*OutputError)
				if !ok || !errors.Is(e, failure) {
					t.Errorf("diagnostic %T %v", p, p)
				}
				if s.closed != 0 {
					t.Error("unexpected close")
				}
			}()
			if offset {
				r.Offset()
			} else {
				r.MustSeek(2, io.SeekCurrent)
			}
			t.Error("missing diagnostic")
		}()
	}
}

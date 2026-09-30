package pgoir

import (
	"bytes"
	"errors"
	"go-backend-ssa/internal/ir"
	"io"
	"strings"
	"testing"
)

type profileStream struct {
	io.Reader
	closed bool
}

func (s *profileStream) Close() error { s.closed = true; return nil }

type failingProfileReader struct{}

func (failingProfileReader) Read([]byte) (int, error) { return 0, io.ErrClosedPipe }

func TestNativeProfileProvider(t *testing.T) {
	c, _ := graphContext()
	c.LookupFunc = func(string) (*ir.Func, error) { return nil, nil }
	c.PostLookupCleanup = func() {}
	if p, e := c.New("missing"); p != nil || e == nil {
		t.Fatal("missing provider accepted")
	}
	for _, data := range []string{"", "GO PREPROFILE V1\nexample/p.F\nremote.F\n3 100\n", "malformed"} {
		r := &profileStream{Reader: strings.NewReader(data)}
		c.OpenProfile = func(name string) (io.ReadCloser, error) {
			if name != "explicit" {
				t.Fatal(name)
			}
			return r, nil
		}
		p, e := c.New("explicit")
		if !r.closed {
			t.Fatal("native input was not closed")
		}
		if data == "malformed" {
			if e == nil || p != nil {
				t.Fatal("malformed input accepted")
			}
			continue
		}
		if e != nil {
			t.Fatal(e)
		}
		if data != "" && (p == nil || !c.OwnsProfile(p) || p.TotalWeight != 100) {
			t.Fatal("profile attribution lost ownership", p)
		}
	}
	r := &profileStream{Reader: failingProfileReader{}}
	c.OpenProfile = func(string) (io.ReadCloser, error) { return r, nil }
	if p, e := c.New("failed"); p != nil || !errors.Is(e, io.ErrClosedPipe) || !r.closed {
		t.Fatal("input failure lost", p, e, r.closed)
	}
	c.OpenProfile = func(string) (io.ReadCloser, error) { return nil, nil }
	if p, e := c.New("nil"); p != nil || e == nil {
		t.Fatal("nil stream accepted")
	}
	c.OpenProfile = func(string) (io.ReadCloser, error) { return nil, io.ErrUnexpectedEOF }
	if p, e := c.New("open"); p != nil || !errors.Is(e, io.ErrUnexpectedEOF) {
		t.Fatal("open failure lost", p, e)
	}
	c.OpenProfile = func(string) (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(nil)), nil }
	if p, e := c.New("next"); p != nil || e != nil {
		t.Fatal("failed parse contaminated subsequent profile", p, e)
	}
}

func TestNativeProfileRejectsForeignLookup(t *testing.T) {
	c, _ := graphContext()
	_, foreign := graphContext()
	r := &profileStream{Reader: strings.NewReader("GO PREPROFILE V1\nexample/p.F\nremote.F\n3 100\n")}
	c.OpenProfile = func(string) (io.ReadCloser, error) { return r, nil }
	c.LookupFunc = func(string) (*ir.Func, error) { return foreign, nil }
	c.PostLookupCleanup = func() {}
	defer func() {
		if recover() == nil {
			t.Error("foreign profile IR accepted")
		}
		if !r.closed {
			t.Error("profile stream left open after rejection")
		}
	}()
	c.New("foreign")
}

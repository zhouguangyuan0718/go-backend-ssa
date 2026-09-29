package pgo_test

import (
	"bufio"
	"bytes"
	"errors"
	"go-backend-ssa/pgo"
	"io"
	"strings"
	"testing"
)

type failedIO struct{ err error }

func (f failedIO) Read([]byte) (int, error)  { return 0, f.err }
func (f failedIO) Write([]byte) (int, error) { return 0, f.err }

func TestNativeProfileStreams(t *testing.T) {
	const data = "GO PREPROFILE V1\na\nb\n-3 90\na\nc\n2 10\n"
	reader := bufio.NewReader(strings.NewReader(data))
	ok, err := pgo.IsSerialized(reader)
	if !ok || err != nil {
		t.Fatal(ok, err)
	}
	p, err := pgo.FromSerialized(reader)
	if err != nil {
		t.Fatal(err)
	}
	var dst bytes.Buffer
	n, err := p.WriteTo(&dst)
	if err != nil || n != int64(len(data)) || dst.String() != data || p.TotalWeight != 100 || pgo.WeightInPercentage(90, 100) != 90 {
		t.Fatal(n, err, dst.String(), p)
	}
	failure := errors.New("stream failure")
	if _, err := pgo.IsSerialized(bufio.NewReader(failedIO{failure})); !errors.Is(err, failure) {
		t.Fatal(err)
	}
	if _, err := pgo.FromSerialized(failedIO{failure}); !errors.Is(err, failure) {
		t.Fatal(err)
	}
	if _, err := p.WriteTo(failedIO{failure}); !errors.Is(err, failure) {
		t.Fatal(err)
	}
	if ok, err := pgo.IsSerialized(bufio.NewReader(strings.NewReader(""))); ok || err != nil {
		t.Fatal(ok, err)
	}
	var _ io.WriterTo = p
}

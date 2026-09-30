package profile

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"testing"
)

func TestOwnedNativeProfileDecoder(t *testing.T) {
	data, err := os.ReadFile("../../../testdata/pgo/hot.pb")
	if err != nil {
		t.Fatal(err)
	}
	for i := range 12 {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			t.Parallel()
			c := NewContext()
			p, err := c.Parse(bytes.NewReader(data))
			if err != nil {
				t.Fatal(err)
			}
			if p.compilation != c || p.SampleType[0].compilation != c || p.Sample[0].compilation != c || p.Location[0].compilation != c || p.Location[0].Line[0].compilation != c || p.Function[0].compilation != c {
				t.Fatal("decoded message lacks session owner")
			}
			if p.Sample[0].Value[0] != 100 || p.Function[1].Name != "p.Caller" {
				t.Fatal("profile data changed")
			}
			var first bytes.Buffer
			if err = p.Write(&first); err != nil {
				t.Fatal(err)
			}
			q, err := NewContext().Parse(bytes.NewReader(first.Bytes()))
			if err != nil {
				t.Fatal(err)
			}
			var second bytes.Buffer
			if err = q.Write(&second); err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(first.Bytes(), second.Bytes()) {
				t.Fatal("native gzip/protobuf round trip differs")
			}
			if _, err = c.Parse(bytes.NewReader(nil)); !errors.Is(err, c.ErrNoData) {
				t.Fatal("empty-input sentinel lost", err)
			} else if errors.Is(err, NewContext().ErrNoData) {
				t.Fatal("error identity shared between sessions")
			}
		})
	}
}

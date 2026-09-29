package pkgbits

import (
	"bytes"
	"errors"
	"fmt"
	"go/constant"
	"go/token"
	"math/big"
	"strings"
	"sync"
	"testing"
)

func encoded(t *testing.T, marker SyncMarker, frames int) string {
	t.Helper()
	pw := NewPkgEncoder(V4, frames)
	w := pw.NewEncoder(SectionMeta, marker)
	w.String("retained")
	w.Flush()
	var b strings.Builder
	pw.DumpTo(&b)
	return b.String()
}

func caught(f func()) (value any) { defer func() { value = recover() }(); f(); return }

func TestDecodeFailureIsolation(t *testing.T) {
	data := encoded(t, SyncPublic, 0)
	old := NewPkgDecoder("old/package", data)
	var wg sync.WaitGroup
	for i := range 16 {
		wg.Go(func() {
			name := fmt.Sprintf("bad/%d", i)
			pr := NewPkgDecoder(name, data)
			var out bytes.Buffer
			pr.TraceWriter = &out
			value := caught(func() { pr.NewDecoder(SectionMeta, PublicRootIdx, SyncPrivate) })
			if abort, ok := value.(DecodeAbort); !ok || abort.Status != 1 {
				t.Errorf("abort: %v", value)
			}
			if !strings.Contains(out.String(), fmt.Sprintf("package %q", name)) || !strings.Contains(out.String(), "expected Private") {
				t.Error(out.String())
			}
			// A failed element read must not poison this decoder or any other instance.
			r := pr.NewDecoder(SectionMeta, PublicRootIdx, SyncPublic)
			if r.String() != "retained" {
				t.Error("read after abort")
			}
		})
	}
	wg.Wait()
	r := old.NewDecoder(SectionMeta, PublicRootIdx, SyncPublic)
	if r.String() != "retained" {
		t.Fatal("old reader changed")
	}
	if _, ok := caught(func() { old.NewDecoder(SectionMeta, PublicRootIdx, SyncPrivate) }).(DecodeAbort); !ok {
		t.Fatal("nil output must still abort")
	}
}

func TestConstantsRelocationsAndScratch(t *testing.T) {
	for _, frames := range []int{-1, 0, 3} {
		t.Run(fmt.Sprint(frames), func(t *testing.T) {
			values := []constant.Value{constant.MakeBool(true), constant.MakeString("世界\x00"), constant.MakeInt64(-42), constant.Make(new(big.Int).Lsh(big.NewInt(1), 180)), constant.Make(new(big.Rat).SetFrac64(-7, 3)), constant.Make(new(big.Float).SetPrec(512).SetFloat64(1.25)), constant.BinaryOp(constant.MakeInt64(2), token.ADD, constant.MakeImag(constant.MakeInt64(3)))}
			pw := NewPkgEncoder(V4, frames)
			for range 3 {
				w := pw.NewEncoder(SectionMeta, SyncPublic)
				w.Len(len(values))
				for _, v := range values {
					w.Value(v)
				}
				w.Strings([]string{"duplicate", "duplicate", "last"})
				w.Int64(-1 << 63)
				w.Uint64(1<<64 - 1)
				w.Flush()
			}
			var out strings.Builder
			fingerprint := pw.DumpTo(&out)
			pr := NewPkgDecoder("constants", out.String())
			if pr.Fingerprint() != fingerprint || pr.NumElems(SectionMeta) != 3 {
				t.Fatal("header")
			}
			for i := range 3 {
				r := pr.TempDecoder(SectionMeta, RelElemIdx(i), SyncPublic)
				if r.Len() != len(values) {
					t.Fatal("value count")
				}
				for _, want := range values {
					if got := r.Value(); got.Kind() != want.Kind() || !constant.Compare(got, token.EQL, want) {
						t.Fatalf("got %s want %s", got.ExactString(), want.ExactString())
					}
				}
				if got := r.Strings(); strings.Join(got, ",") != "duplicate,duplicate,last" {
					t.Fatal(got)
				}
				if r.Int64() != -1<<63 || r.Uint64() != 1<<64-1 {
					t.Fatal("integer bounds")
				}
				pr.RetireDecoder(&r)
				if r.Relocs != nil {
					t.Fatal("retired decoder holds scratch")
				}
			}
		})
	}
}

func TestMalformedInputAndOverflowIdentity(t *testing.T) {
	data := encoded(t, SyncPublic, -1)
	for _, input := range []string{"", data[:7], string([]byte{255, 255, 255, 255})} {
		if caught(func() { NewPkgDecoder("malformed", input) }) == nil {
			t.Fatal("malformed input accepted")
		}
	}
	a, b := NewPkgDecoder("a", data), NewPkgDecoder("b", data)
	if a.overflow == b.overflow {
		t.Fatal("shared overflow sentinel")
	}
	for range 2 {
		_, err := a.readUvarint(strings.NewReader(strings.Repeat("\xff", 10)))
		if !errors.Is(err, a.overflow) {
			t.Fatalf("sentinel identity: %v", err)
		}
	}
}

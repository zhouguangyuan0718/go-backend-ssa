package target_test

import (
	"go-backend-ssa/target"
	"go/types"
	"testing"
)

func TestLayoutsMatchReferenceAndRemainIndependent(t *testing.T) {
	for _, arch := range []string{"amd64", "arm64", "386"} {
		t.Run(arch, func(t *testing.T) {
			t.Parallel()
			l, err := target.NewLayout("linux", arch)
			if err != nil {
				t.Fatal(err)
			}
			ptr := types.NewPointer(types.Typ[types.Uint8])
			want := types.SizesFor("gc", arch).Sizeof(ptr)
			if int64(l.PointerSize()) != want || l.RegisterSize() != l.PointerSize() {
				t.Fatalf("bad target layout: %+v", l)
			}
			_, _ = target.NewLayout("linux", "386")
			if int64(l.PointerSize()) != want {
				t.Fatal("another target changed existing layout")
			}
		})
	}
	if (target.Layout{}).Valid() {
		t.Fatal("zero target accepted")
	}
	for _, v := range [][2]string{{"", ""}, {"darwin", "amd64"}, {"linux", "unknown"}} {
		if _, err := target.NewLayout(v[0], v[1]); err == nil {
			t.Fatal("unsupported target accepted")
		}
	}
}

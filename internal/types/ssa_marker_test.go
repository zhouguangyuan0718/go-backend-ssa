package types

import (
	"fmt"
	"testing"
)

func TestNativePackageLessSSAMarkerFormatting(t *testing.T) {
	for i := range 8 {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			t.Parallel()
			a, b := &Sym{Name: "mem"}, &Sym{Name: "mem"}
			for _, format := range []string{"%v", "%S", "%+v", "%#v"} {
				text := fmt.Sprintf(format, a)
				if text != "mem" {
					t.Errorf("%s: %s", format, text)
				}
			}
			if a == b || a.Pkg != nil || b.Pkg != nil || a.Universe() != nil {
				t.Fatal("marker acquired package identity")
			}
			if got := fmt.Sprint((*Sym)(nil)); got != "<S>" {
				t.Error(got)
			}
		})
	}
	defer func() {
		if recover() == nil {
			t.Error("unowned non-nil package accepted")
		}
	}()
	formatUniverse(&Sym{Name: "bad", Pkg: &Pkg{}})
}

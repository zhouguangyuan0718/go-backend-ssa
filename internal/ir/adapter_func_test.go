package ir

import (
	"go-backend-ssa/internal/cmd/objabi"
	"testing"
)

func TestLinkNamesRoundTrip(t *testing.T) {
	for _, pkg := range []string{"fmt", "example.com/project/pkg.v2", "example.com/包", "path/with space"} {
		for _, sym := range []string{"F", "(*T).M", "F[sync/atomic.Uint64]"} {
			name := objabi.PathToPrefix(pkg) + "." + sym
			p, s, err := ParseLinkFuncName(name)
			if err != nil || p != pkg || s != sym {
				t.Fatalf("%q: %q %q %v", name, p, s, err)
			}
		}
	}
	for _, bad := range []string{"NoPackage", "bad%xx.F"} {
		if _, _, err := ParseLinkFuncName(bad); err == nil {
			t.Fatalf("accepted malformed %q", bad)
		}
	}
}

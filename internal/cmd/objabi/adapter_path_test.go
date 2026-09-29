package objabi

import "testing"

func TestEncodingPreservesImportPaths(t *testing.T) {
	for _, p := range []string{"", "normal/path", "example.com/pkg.v2", "x/%quote\"", "非ASCII", "a\x00b", "a b"} {
		got, err := PrefixToPath(PathToPrefix(p))
		if err != nil || got != p {
			t.Fatalf("round trip %q: %q %v", p, got, err)
		}
	}
	for _, bad := range []string{"%", "%0", "%gg"} {
		if _, err := PrefixToPath(bad); err == nil {
			t.Fatalf("accepted %q", bad)
		}
	}
}

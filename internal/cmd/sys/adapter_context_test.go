package sys

import "testing"

func TestNativeArchitectureIdentity(t *testing.T) {
	a, b := NewContext(), NewContext()
	seen := map[string]bool{}
	for i, x := range a.Archs {
		y := b.Archs[i]
		if x == y || ContextOfArch(x) != a || ContextOfArch(y) != b || x.Name != y.Name || seen[x.Name] {
			t.Fatal("architecture identity or ownership changed")
		}
		seen[x.Name] = true
		if !x.InFamily(x.Family) {
			t.Fatal("native family query")
		}
	}
	if len(seen) != 14 || a.Archs[11] != a.ArchRISCV64 || b.Archs[11] != b.ArchRISCV64 {
		t.Fatal("canonical descriptor table")
	}
	old := b.ArchRISCV64.MinLC
	a.ArchRISCV64.MinLC = 99
	if b.ArchRISCV64.MinLC != old {
		t.Fatal("descriptor shared across instances")
	}
	clone := *a.ArchRISCV64
	if &clone == a.ArchRISCV64 || ContextOfArch(&clone) != a {
		t.Fatal("copy acquired canonical pointer identity")
	}
	if ContextOfArch(nil) != nil || ContextOfArch(&Arch{}) != nil {
		t.Fatal("caller-built descriptor acquired an owner")
	}
}

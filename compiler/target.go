package compiler

import (
	"fmt"
	"go-backend-ssa/internal/go/buildcfg"
)

// Normalize CPU features using the pinned native parser. Only the selected
// architecture's input is accepted, so metadata has one unambiguous identity.
func normalizeTarget(t Target) (Target, buildcfg.Goarm64Features, error) {
	var arm buildcfg.Goarm64Features
	invalid := func() (Target, buildcfg.Goarm64Features, error) {
		return t, arm, fmt.Errorf("unsupported native target: %+v", t)
	}
	if t.GOOS != "linux" {
		return invalid()
	}
	switch t.GOARCH {
	case "amd64":
		if t.GOAMD64 < 1 || t.GOAMD64 > 4 || t.GOARM64 != "" {
			return invalid()
		}
	case "arm64":
		if t.GOAMD64 != 0 {
			return invalid()
		}
		if t.GOARM64 == "" {
			t.GOARM64 = "v8.0"
		}
		var err error
		arm, err = (&buildcfg.Context{DefaultGOARM64: "v8.0"}).ParseGoarm64(t.GOARM64)
		if err != nil {
			return t, arm, err
		}
		t.GOARM64 = arm.String()
	default:
		return invalid()
	}
	return t, arm, nil
}

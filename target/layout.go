// Package target defines explicit target layout inputs, independent of the host.
// Layout support alone does not imply a working compiler or backend for a target.
package target

import "fmt"

// Layout is an immutable value. Its zero value is invalid. It describes the
// initial Linux amd64/arm64 targets plus 386 for 32-bit layout verification.
// It is not a type-size calculator; that will use the ported types.Universe.
type Layout struct {
	arch             string
	ptrSize, regSize int
	maxWidth         int64
}

func NewLayout(goos, goarch string) (Layout, error) {
	if goos != "linux" {
		return Layout{}, fmt.Errorf("unsupported target %q/%q", goos, goarch)
	}
	switch goarch {
	case "amd64", "arm64":
		return Layout{goarch, 8, 8, 1 << 50}, nil
	case "386":
		return Layout{goarch, 4, 4, (1 << 32) - 1}, nil
	default:
		return Layout{}, fmt.Errorf("unsupported target %q/%q", goos, goarch)
	}
}

func (l Layout) Valid() bool { return l.arch != "" }
func (l Layout) GOOS() string {
	if !l.Valid() {
		return ""
	}
	return "linux"
}
func (l Layout) GOARCH() string    { return l.arch }
func (l Layout) PointerSize() int  { return l.ptrSize }
func (l Layout) RegisterSize() int { return l.regSize }
func (l Layout) MaxWidth() int64   { return l.maxWidth }

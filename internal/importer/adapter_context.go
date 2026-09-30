package importer

import (
	"fmt"
	"go-backend-ssa/internal/cmd/src"
	"go-backend-ssa/internal/go/pkgbits"
	"go-backend-ssa/internal/types2"
)

// Context binds the native lazy importer to one checker universe. Operations
// and lazy lookups within this graph are serial; separate graphs are independent.
type ImportContext struct {
	checker  *types2.Context
	packages map[string]*types2.Package
}

func NewContext(checker *types2.Context) *ImportContext {
	if checker == nil {
		panic("importer requires a checker context")
	}
	return &ImportContext{checker: checker, packages: make(map[string]*types2.Package)}
}
func (c *ImportContext) checkRead(checker *types2.Context, packages map[string]*types2.Package, input pkgbits.PkgDecoder) {
	if c == nil || checker == nil || checker != c.checker || packages == nil {
		panic("importer requires this checker's package map")
	}
	for path, pkg := range packages {
		if pkg == nil || pkg.Context() != checker || pkg.Path() != path {
			panic("imported package belongs to another or uninitialized checker")
		}
	}
}
func (c *ImportContext) fatalf(format string, args ...any)                 { panic(fmt.Errorf(format, args...)) }
func (c *ImportContext) fatalfAt(pos src.XPos, format string, args ...any) { c.fatalf(format, args...) }
func (c *ImportContext) assertCondition(ok bool) {
	if !ok {
		c.fatalf("assertion failed")
	}
}
func (c *ImportContext) assertf(ok bool, format string, args ...any) {
	if !ok {
		c.fatalf(format, args...)
	}
}

// ReadExport decodes one finalized native Unified IR payload into this checker's
// package graph. The payload excludes archive/object headers and the leading u.
// Load each package once. Transitive declarations are loaded lazily and share
// identities when that dependency's own export payload is subsequently loaded.
// Invalid native data panics; discard this graph after such a failure.
func (c *ImportContext) ReadExport(path, data string) *types2.Package {
	if c == nil || c.checker == nil || path == "" || path == "unsafe" || path == "builtin" {
		panic("importer requires a non-builtin package path")
	}
	if pkg := c.packages[path]; pkg != nil && pkg.Complete() {
		panic("package export already loaded")
	}
	decoder := pkgbits.NewPkgDecoder(path, data)
	decoder.TraceWriter = c.checker.TraceWriter
	return c.ReadPackage(c.checker, c.packages, decoder)
}

// Import implements types2.Importer for already-loaded complete dependencies.
// It performs no filesystem, network or toolchain lookup.
func (c *ImportContext) Import(path string) (*types2.Package, error) {
	if c == nil || c.checker == nil {
		return nil, fmt.Errorf("uninitialized importer")
	}
	if path == "unsafe" {
		return c.checker.Unsafe, nil
	}
	if pkg := c.packages[path]; pkg != nil && pkg.Complete() {
		return pkg, nil
	}
	return nil, fmt.Errorf("package %q has no loaded export data", path)
}

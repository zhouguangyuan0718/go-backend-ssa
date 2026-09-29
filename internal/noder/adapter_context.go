package noder

import (
	"fmt"
	"go-backend-ssa/diagnostic"
	errors "go-backend-ssa/internal/go/types/errors"
	"go-backend-ssa/internal/ir"
	"go-backend-ssa/internal/types"
	"go-backend-ssa/internal/types2"
	"go/version"
	"io"

	"go-backend-ssa/internal/base"
	"go-backend-ssa/internal/cmd/obj"
	"go-backend-ssa/internal/cmd/objabi"
	"go-backend-ssa/internal/cmd/src"
	"go-backend-ssa/internal/syntax"
)

// Context owns the formerly process-wide inputs used by the selected noder
// algorithms. The supplied Link and its position table belong to this instance.
type Context struct {
	checked       *types2.Package
	info          *types2.Info
	rangeClosures map[*syntax.FuncLit]bool

	Ctxt                                          *obj.Link
	Flag                                          base.CmdFlags
	GOROOT                                        string
	Checker                                       *types2.Context
	Types                                         *types.Universe
	Debug                                         base.DebugFlags
	GOARCH                                        string
	TraceWriter                                   io.Writer
	Diagnostics                                   diagnostic.Collector
	anyTypeName, comparableTypeName, runeTypeName *types2.TypeName
	unOps                                         [syntax.And + 1]ir.Op
	binOps                                        [syntax.Shr + 1]ir.Op
}

func (c *Context) absFile(dir, file, rewrites string) string {
	paths := objabi.PathContext{GOROOT: c.GOROOT}
	return paths.AbsFile(dir, file, rewrites)
}

type PosMap = posMap

// NewPosMap maps parser positions into the provided native Link position table.
// No cwd/GOROOT/environment lookup is performed. Do not copy an active map.
func NewPosMap(link *obj.Link, trimPath, goroot string) *PosMap {
	if link == nil {
		panic("nil native Link")
	}
	c := &Context{}
	c.Ctxt = link
	c.Flag.TrimPath = trimPath
	c.GOROOT = goroot
	return &posMap{compilation: c}
}
func (m *posMap) XPos(pos syntax.Pos) src.XPos             { return m.makeXPos(pos) }
func (m *posMap) TrimFilename(base *syntax.PosBase) string { return m.compilation.trimFilename(base) }

type discardTrace struct{}

func (discardTrace) Write(p []byte) (int, error) { return len(p), nil }
func (c *Context) traceWriter() io.Writer {
	if c.TraceWriter != nil {
		return c.TraceWriter
	}
	return discardTrace{}
}
func (c *Context) errorfAt(pos src.XPos, code errors.Code, format string, args ...any) {
	c.Diagnostics.Add(diagnostic.Error, code.String(), c.Ctxt.PosTable.Pos(pos), fmt.Sprintf(format, args...))
}
func (c *Context) fatalf(format string, args ...any) { panic(fmt.Errorf(format, args...)) }
func (c *Context) fatalfAt(pos src.XPos, format string, args ...any) {
	panic(fmt.Errorf("%s: %s", c.Ctxt.PosTable.Pos(pos), fmt.Sprintf(format, args...)))
}
func (c *Context) assertCondition(ok bool) {
	if !ok {
		c.fatalf("assertion failed")
	}
}
func (c *Context) assertf(ok bool, format string, args ...any) {
	if !ok {
		c.fatalf(format, args...)
	}
}
func (c *Context) allowsGoVersion(major, minor int) bool {
	return c.Flag.Lang == "" || version.Compare(c.Flag.Lang, fmt.Sprintf("go%d.%d", major, minor)) >= 0
}
func (c *Context) tracePrintf(format string, args ...any) (int, error) {
	return fmt.Fprintf(c.traceWriter(), format, args...)
}
func (c *Context) tracePrintln(args ...any) (int, error) {
	return fmt.Fprintln(c.traceWriter(), args...)
}
func (c *Context) tracePrint(args ...any) (int, error) { return fmt.Fprint(c.traceWriter(), args...) }

// NewContext binds native checker, compiler type and object instances. Inputs
// remain caller-owned and must not be mutated concurrently with writing.
func NewContext(checker *types2.Context, compilerTypes *types.Universe, link *obj.Link) (*Context, error) {
	if checker == nil || compilerTypes == nil || link == nil || link.Arch == nil || link.Arch.Arch == nil {
		return nil, fmt.Errorf("noder requires checker, compiler types and native target Link")
	}
	if link.Arch.PtrSize != compilerTypes.PointerSize() || link.Arch.RegSize != compilerTypes.RegisterSize() {
		return nil, fmt.Errorf("noder target layout mismatch")
	}
	c := &Context{Checker: checker, Types: compilerTypes, Ctxt: link, GOARCH: link.Arch.Name}
	c.Flag.Lang = "go1.27"
	c.Flag.Complete = true
	c.Debug.SyncFrames = -1
	return c, nil
}

// WritePackage runs the upstream package-stub writer on already checked ASTs.
// Info must contain the compiler maps and StoreTypesInSyntax results. Frontend
// rewrites (rangefunc/midway), directive collection, imports, and diagnostics
// belong to the caller at this low-level boundary. No type graph is cleared.
func (c *Context) WritePackage(pkg *types2.Package, info *types2.Info, files []*syntax.File, rangeClosures map[*syntax.FuncLit]bool) (data string, err error) {
	if c == nil || c.Checker == nil || c.Types == nil || c.Ctxt == nil {
		return "", fmt.Errorf("uninitialized noder context")
	}
	if pkg == nil || pkg.Context() != c.Checker || info == nil || !info.StoreTypesInSyntax || info.Defs == nil || info.Uses == nil || info.Instances == nil || info.Selections == nil || info.Implicits == nil || info.Scopes == nil || info.FileVersions == nil {
		return "", fmt.Errorf("noder requires this checker's package and complete compiler Info")
	}
	if len(files) == 0 {
		return "", fmt.Errorf("noder requires source files")
	}
	for _, file := range files {
		if file == nil {
			return "", fmt.Errorf("nil source file")
		}
		syntax.Inspect(file, func(node syntax.Node) bool {
			if clause, ok := node.(*syntax.RangeClause); ok {
				typ := clause.X.GetTypeInfo().Type
				if typ != nil {
					if _, ok := c.Checker.CoreType(typ).(*types2.Signature); ok {
						err = fmt.Errorf("range-over-func requires native frontend rewriting before package writing")
					}
				}
			}
			return true
		})
	}
	if err != nil {
		return "", err
	}
	c.Diagnostics = diagnostic.Collector{}
	c.checked, c.info, c.rangeClosures = pkg, info, rangeClosures
	defer func() { c.checked = nil; c.info = nil; c.rangeClosures = nil }()
	c.initContext()
	noders := make([]*noder, len(files))
	for i, file := range files {
		noders[i] = &noder{file: file}
	}
	data = c.writePkgStub(posMap{compilation: c}, noders)
	if c.Diagnostics.HasErrors() {
		return "", fmt.Errorf("noder: %s", c.Diagnostics.Diagnostics()[0])
	}
	return data, nil
}
func (c *Context) checkedFiles(m posMap, noders []*noder) (*types2.Package, *types2.Info, map[*syntax.FuncLit]bool) {
	if m.compilation != c || c.checked == nil {
		panic("unbound checked package")
	}
	return c.checked, c.info, c.rangeClosures
}
func (c *Context) retainCheckedPackage(pkg *types2.Package) {
	if pkg != c.checked {
		panic("unexpected package at writer lifetime boundary")
	}
	// The command driver clears this graph to force early collection. The library
	// must preserve objects retained by its caller, so it deliberately does not.
}

func (c *Context) isGlobal(obj types2.Object) bool {
	return obj.Parent() == c.Checker.PackageScope(obj.Pkg())
}

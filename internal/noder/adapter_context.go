package noder

import (
	"fmt"
	"go-backend-ssa/diagnostic"
	errors "go-backend-ssa/internal/go/types/errors"
	"go-backend-ssa/internal/ir"
	"go-backend-ssa/internal/rangefunc"
	"go-backend-ssa/internal/reflectdata"
	"go-backend-ssa/internal/staticinit"
	"go-backend-ssa/internal/typecheck"
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
	StaticInit                                       *staticinit.Context
	ReflectData                                      *reflectdata.Context
	ptrSize, renameinitgen, inlgen                   int
	basics                                           [types2.UntypedNil + 1]**types.Type
	dirs                                             [3]types.ChanDir
	localPkgReader                                   *pkgReader
	objReader                                        map[*types.Sym]pkgReaderIndex
	bodyReader                                       map[*ir.Func]pkgReaderIndex
	importBodyReader                                 map[*types.Sym]pkgReaderIndex
	todoDicts                                        []func()
	todoBodies                                       []*ir.Func
	needWrapperTypes, haveWrapperTypes               []*types.Type
	needMethodValueWrappers, haveMethodValueWrappers []methodValueWrapper

	Base          *base.LinkContext
	checked       *types2.Package
	info          *types2.Info
	rangeClosures map[*syntax.FuncLit]bool

	Ctxt                                          *obj.Link
	Flag                                          *base.CmdFlags
	GOROOT                                        string
	Checker                                       *types2.Context
	Types                                         *types.Universe
	Debug                                         *base.DebugFlags
	GOARCH                                        string
	Typecheck                                     *typecheck.Context
	Diagnostics                                   *diagnostic.Collector
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
	c := &Context{Base: base.NewLinkContext(link)}
	c.Flag = &c.Base.Flag
	c.Ctxt = link
	c.Flag.TrimPath = trimPath
	c.GOROOT = goroot
	return &posMap{compilation: c}
}
func (m *posMap) XPos(pos syntax.Pos) src.XPos             { return m.makeXPos(pos) }
func (m *posMap) TrimFilename(base *syntax.PosBase) string { return m.compilation.trimFilename(base) }

func (c *Context) traceWriter() io.Writer {
	if c.Typecheck != nil && c.Typecheck.IR.TraceWriter != nil {
		return c.Typecheck.IR.TraceWriter
	}
	return io.Discard
}
func (c *Context) warn(format string, args ...any) {
	c.Diagnostics.Add(diagnostic.Warning, "", c.Ctxt.PosTable.Pos(c.Types.Pos), fmt.Sprintf(format, args...))
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

// NewContext binds the source checker to the existing native IR/typecheck graph.
// Flags, debugging and diagnostics are shared with that graph. Initialize its
// compiler universe first; configuration remains owned by the caller.
func NewContext(checker *types2.Context, tc *typecheck.Context) (*Context, error) {
	if checker == nil || tc == nil || tc.IR == nil || tc.IR.Base == nil || tc.IR.Types == nil || tc.IR.Base.Ctxt == nil {
		return nil, fmt.Errorf("noder requires a checker and native IR/typecheck instance")
	}
	graph := tc.IR
	u, link := graph.Types, graph.Base.Ctxt
	if link.Arch == nil || link.Arch.Arch == nil || u.PosTable != &link.PosTable || link.Arch.PtrSize != u.PointerSize() || link.Arch.RegSize != u.RegisterSize() {
		return nil, fmt.Errorf("noder target/position mismatch")
	}
	if u.Types[types.TINT] == nil || u.Types[types.TINT].Obj() == nil {
		return nil, fmt.Errorf("noder requires initialized native compiler types")
	}
	c := &Context{Base: graph.Base, Checker: checker, Typecheck: tc, Types: u, Ctxt: link, GOARCH: link.Arch.Name, Flag: &graph.Base.Flag, Debug: &graph.Debug, Diagnostics: &graph.Diagnostics}
	c.ptrSize = u.PointerSize()
	c.initContext()
	return c, nil
}
func (c *Context) checkShape(targ *types.Type, basic bool) {
	if targ == nil || targ.Universe() != c.Types {
		panic("shape type belongs to another or uninitialized compilation")
	}
}
func (c *Context) checkDictArgs(sym *types.Sym, idx index, implicits, explicits []*types.Type, shaped bool) {
	if sym == nil || sym.Universe() != c.Types {
		panic("dictionary symbol belongs to another or uninitialized compilation")
	}
	for _, args := range [][]*types.Type{implicits, explicits} {
		for _, typ := range args {
			c.checkShape(typ, false)
		}
	}
}

// validateCheckedPackage checks the low-level input ownership and Info contract.
func (c *Context) validateCheckedPackage(pkg *types2.Package, info *types2.Info, files []*syntax.File) error {
	if c == nil || c.Checker == nil || c.Types == nil || c.Ctxt == nil {
		return fmt.Errorf("uninitialized noder context")
	}
	if pkg == nil || pkg.Context() != c.Checker || info == nil || !info.StoreTypesInSyntax || info.Defs == nil || info.Uses == nil || info.Instances == nil || info.Selections == nil || info.Implicits == nil || info.Scopes == nil || info.FileVersions == nil {
		return fmt.Errorf("noder requires this checker's package and complete compiler Info")
	}
	if len(files) == 0 {
		return fmt.Errorf("noder requires source files")
	}
	for _, file := range files {
		if file == nil {
			return fmt.Errorf("nil source file")
		}
	}
	return nil
}

// RewriteRangeFuncs runs the original rangefunc pass on successfully checked
// ASTs, updating them and Info in place. Call exactly once, before WritePackage,
// and pass the returned closure map to the writer. Existing package types stay
// valid; callers must not concurrently read the AST or Info being rewritten.
func (c *Context) RewriteRangeFuncs(pkg *types2.Package, info *types2.Info, files []*syntax.File) (map[*syntax.FuncLit]bool, error) {
	if err := c.validateCheckedPackage(pkg, info, files); err != nil {
		return nil, err
	}
	r := rangefunc.NewContext(c.Checker)
	r.Flag, r.Debug, r.TraceWriter = *c.Flag, *c.Debug, c.traceWriter()
	return r.Rewrite(pkg, info, files), nil
}

// WritePackage runs the native writer after required frontend preparation.
// See RewriteRangeFuncs for range-over-function source. Compiler directives and
// experimental midway rewriting are not yet integrated at this entry point.
func (c *Context) WritePackage(pkg *types2.Package, info *types2.Info, files []*syntax.File, rangeClosures map[*syntax.FuncLit]bool) (data string, err error) {
	if err := c.validateCheckedPackage(pkg, info, files); err != nil {
		return "", err
	}
	for _, file := range files {
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
	diagnosticStart := len(c.Diagnostics.Diagnostics())
	c.checked, c.info, c.rangeClosures = pkg, info, rangeClosures
	defer func() { c.checked = nil; c.info = nil; c.rangeClosures = nil }()
	noders := make([]*noder, len(files))
	for i, file := range files {
		noders[i] = &noder{file: file}
	}
	data = c.writePkgStub(posMap{compilation: c}, noders)
	for _, d := range c.Diagnostics.Diagnostics()[diagnosticStart:] {
		if d.Severity == diagnostic.Error {
			return "", fmt.Errorf("noder: %s", d)
		}
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

func (c *Context) assertfAt(ok bool, pos src.XPos, format string, args ...any) {
	if !ok {
		c.fatalfAt(pos, format, args...)
	}
}

// Bind the native marker at the first scope without replacing live scope data.
func scopeReaderContext(r *reader) *Context {
	if r == nil || r.compilation == nil {
		panic("reader has no compilation owner")
	}
	r.marker.Bind(r.compilation.Base)
	return r.compilation
}

func (c *Context) warnfAt(pos src.XPos, format string, args ...any) {
	c.Diagnostics.Add(diagnostic.Warning, "", c.Ctxt.PosTable.Pos(pos), fmt.Sprintf(format, args...))
}

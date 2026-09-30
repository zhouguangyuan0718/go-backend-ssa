package noder

import (
	"fmt"
	"go-backend-ssa/diagnostic"
	"go-backend-ssa/internal/go/buildcfg"
	"go-backend-ssa/internal/go/pkgbits"
	errors "go-backend-ssa/internal/go/types/errors"
	"go-backend-ssa/internal/importer"
	"go-backend-ssa/internal/inline"
	"go-backend-ssa/internal/inline/inlheur"
	"go-backend-ssa/internal/inline/interleaved"
	"go-backend-ssa/internal/ir"
	"go-backend-ssa/internal/logopt"
	"go-backend-ssa/internal/midway"
	"go-backend-ssa/internal/pgoir"
	"go-backend-ssa/internal/rangefunc"
	"go-backend-ssa/internal/reflectdata"
	"go-backend-ssa/internal/staticinit"
	"go-backend-ssa/internal/typecheck"
	"go-backend-ssa/internal/types"
	"go-backend-ssa/internal/types2"
	"go/version"
	"io"
	"io/fs"
	"reflect"
	"regexp"
	"strings"
	"sync"

	"go-backend-ssa/internal/base"
	"go-backend-ssa/internal/cmd/obj"
	"go-backend-ssa/internal/cmd/objabi"
	"go-backend-ssa/internal/cmd/src"
	"go-backend-ssa/internal/syntax"
)

// Context owns the formerly process-wide inputs used by the selected noder
// algorithms. The supplied Link and its position table belong to this instance.
type Context struct {
	versionErrorRx            *regexp.Regexp
	sourceStarted, sourceMode bool
	syntaxMu                  sync.Mutex
	syntaxHTMLWriters         map[*syntax.FuncDecl]*HTMLWriter
	syntaxOrderedFuncs        []*syntax.FuncDecl
	syntaxNodeType            reflect.Type
	OpenSyntaxHTML            func(string, int, fs.FileMode) (io.WriteCloser, error)
	SyntaxWorkingDirectory    func() (string, error)

	importHeader                *objabi.Context
	hostOS                      string
	dependencyOpen              func(string) (DependencyFile, error)
	typeImports                 *importer.ImportContext
	importPackages              map[string]*types2.Package
	importChecker               *types2.Context
	importStarted, importFailed bool

	exportDone                                       bool
	allowedStdPragmas                                map[string]bool
	GOOS                                             string
	Interleaved                                      *interleaved.Context
	Inline                                           *inline.Context
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
	paths := objabi.Context{GOROOT: c.GOROOT}
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
	c.initializeWriterUniverse()
	tc.HaveInlineBody = c.unifiedHaveInlineBody
	c.Inline = inline.NewContext(tc, inlheur.NewContext(tc, pgoir.NewContext(tc)), logopt.NewContext())
	c.Inline.PGO.PostLookupCleanup = c.PostLookupCleanup
	c.Inline.PGO.LookupFunc = c.LookupFunc
	c.Inline.InlineCall = c.unifiedInlineCall
	c.Interleaved = interleaved.NewContext(c.Inline)
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
func (c *Context) WritePackage(pkg *types2.Package, info *types2.Info, files []*syntax.File, rangeClosures map[*syntax.FuncLit]bool) (string, error) {
	noders := make([]*noder, len(files))
	for i, file := range files {
		noders[i] = &noder{compilation: c, file: file}
	}
	return c.writeCheckedPackage(pkg, info, files, noders, rangeClosures)
}

func (c *Context) writeCheckedPackage(pkg *types2.Package, info *types2.Info, files []*syntax.File, noders []*noder, rangeClosures map[*syntax.FuncLit]bool) (data string, err error) {
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
	data = c.writePkgStub(posMap{compilation: c}, noders)
	for _, d := range c.Diagnostics.Diagnostics()[diagnosticStart:] {
		if d.Severity == diagnostic.Error {
			return "", fmt.Errorf("noder: %s", d)
		}
	}
	return data, nil
}
func (c *Context) checkedFiles(m posMap, noders []*noder) (*types2.Package, *types2.Info, map[*syntax.FuncLit]bool) {
	if m.compilation != c {
		panic("foreign source position map")
	}
	if c.sourceMode {
		c.checked, c.info, c.rangeClosures = c.checkFiles(m, noders)
	}
	if c.checked == nil {
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

// ReadBodies runs the native pending dictionary/body driver for this package.
// During inlining, imported generic functions are analyzed for inlinability and
// their bodies released using the upstream policy; linkshared methods are kept.
func (c *Context) ReadBodies(duringInlining bool, profile *pgoir.Profile) {
	if profile != nil && !c.Inline.PGO.OwnsProfile(profile) {
		panic("body profile belongs to another compilation")
	}
	c.readBodies(c.Typecheck.Target, duringInlining, profile)
}

// RegisterPackage registers the native unified bitstream in this compilation.
// localStub selects WritePackage's local stub; false selects finalized import
// data (without the archive/export header). Register each package once. Import
// declarations and inline bodies are decoded lazily by the native reader.
// Invalid data or compiler diagnostics abort with a panic, never process exit.
func (c *Context) RegisterPackage(pkg *types.Pkg, data string, localStub bool) {
	if pkg == nil || pkg.Universe() != c.Types {
		panic("registered package belongs to another compilation")
	}
	if localStub && (pkg != c.Types.LocalPkg || c.localPkgReader != nil) {
		panic("local package stub must be registered once for the compilation's local package")
	}
	decoder := pkgbits.NewPkgDecoder(pkg.Path, data)
	decoder.TraceWriter = c.traceWriter()
	pr := c.newPkgReader(decoder)
	c.readPackage(pr, pkg, localStub)
	if localStub {
		c.localPkgReader = pr
	}
}

// ReadLocalPackage performs the reader portion of unified on a local stub:
// register its objects, read package initialization, and drain dictionaries and
// function bodies. It returns the existing native Target. Configure runtime,
// reflection and static initialization on this graph before calling. Wrapper
// generation and the complete unified/optimization driver are not run here.
func (c *Context) ReadLocalPackage(data string) *ir.Package {
	if c.ReflectData == nil || c.ReflectData.Typecheck != c.Typecheck || c.StaticInit == nil || c.StaticInit.Typecheck != c.Typecheck || c.StaticInit.ReflectData != c.ReflectData {
		panic("local package reading requires shared reflection and static initialization contexts")
	}
	c.RegisterPackage(c.Types.LocalPkg, data, true)
	c.localPkgReader.newReader(pkgbits.SectionMeta, pkgbits.PrivateRootIdx, pkgbits.SyncPrivate).pkgInit(c.Types.LocalPkg, c.Typecheck.Target)
	c.ReadBodies(false, nil)
	return c.Typecheck.Target
}

func (c *Context) errorExit() {
	if c.sourceMode && c.Diagnostics.HasErrors() {
		panic(SourceErrors(c.Diagnostics.Diagnostics()))
	}
	for _, d := range c.Diagnostics.Diagnostics() {
		if d.Severity == diagnostic.Error {
			panic(fmt.Errorf("noder: %s", d))
		}
	}
	panic("noder: compilation aborted")
}

// MakeWrappers is a native, once-per-package phase after package inlining.
// Validate the host graph before native symbol flags or queues are changed.
func (c *Context) checkWrapperTarget(target *ir.Package) {
	if target == nil || target != c.Typecheck.Target {
		panic("wrapper package is not this compilation's target")
	}
	if c.ReflectData == nil || c.ReflectData.Typecheck != c.Typecheck || c.Interleaved == nil || c.Interleaved.Inline != c.Inline {
		panic("wrapper generation requires shared reflection and inlining contexts")
	}
	for _, fn := range target.Funcs {
		if fn == nil || !c.Typecheck.IR.OwnsNode(fn) {
			panic("wrapper input belongs to another compilation")
		}
	}
}

// checkExport protects the native reader relocation caches and ensures exported
// symbol indices come from completed object numbering.
func (c *Context) checkExport(out io.Writer) {
	if c == nil || out == nil || !c.ExportAvailable() || !c.Ctxt.ObjectNumbered() || c.Ctxt.ObjectWritten() {
		panic("export requires a local package and unconsumed numbered symbols")
	}
	c.exportDone = true
}

// ExportUnified writes the native finalized Unified IR bitstream and updates
// the Link fingerprint. Number symbols first; this phase is single-use.
func (c *Context) ExportUnified(out io.Writer) (err error) {
	if out == nil {
		return fmt.Errorf("export output is nil")
	}
	defer func() {
		if p := recover(); p != nil {
			if failure, ok := p.(*exportOutputFailure); ok {
				err = failure.cause
			} else {
				panic(p)
			}
		}
	}()
	c.writeUnifiedExport(exportOutput{out})
	return nil
}

type exportOutput struct{ io.Writer }
type exportOutputFailure struct{ cause error }

func (w exportOutput) Write(p []byte) (int, error) {
	n, err := w.Writer.Write(p)
	if err == nil && n != len(p) {
		err = io.ErrShortWrite
	}
	if err != nil {
		panic(&exportOutputFailure{err})
	}
	return n, nil
}

// ExportAvailable reports whether a local reader has an unused export phase.
func (c *Context) ExportAvailable() bool {
	return c != nil && c.localPkgReader != nil && !c.exportDone && c.Ctxt != nil
}

// ParsedFile retains the native per-file directives with their syntax tree.
// The tree may be checked and rewritten by this Context's checker; directives
// must travel with it to WriteParsedPackage. Do not copy or mutate this handle.
type ParsedFile struct{ native *noder }

func (f *ParsedFile) Syntax() *syntax.File {
	if f == nil || f.native == nil {
		return nil
	}
	return f.native.file
}

// ParseErrors preserves all native parsing/directive errors in source order.
type ParseErrors []syntax.Error

func (e ParseErrors) Error() string {
	if len(e) == 0 {
		return "noder: parse failed"
	}
	return e[0].Error()
}

// ParseFile runs the original syntax parser with the original noder directive
// handlers. Inputs are explicit; this method does not open files. Configure
// GOOS, flags, target, package path and experiments before parsing. Multiple
// files may be parsed concurrently while that configuration remains fixed.
// A parser or reader panic is propagated after the error collector is joined.
func (c *Context) ParseFile(filename string, input io.Reader) (*ParsedFile, error) {
	if c == nil || c.Ctxt == nil || c.Types == nil || c.Base == nil || input == nil || filename == "" || c.GOOS == "" || c.GOARCH == "" || c.Ctxt.Pkgpath == "" {
		return nil, fmt.Errorf("noder parsing requires a source, filename, target OS/architecture and package path")
	}
	p := &noder{compilation: c, err: make(chan syntax.Error)}
	var failures ParseErrors
	done := make(chan struct{})
	go func() {
		defer close(done)
		for e := range p.err {
			failures = append(failures, e)
		}
	}()
	func() {
		defer func() { close(p.err); <-done }()
		p.file, _ = syntax.Parse(syntax.NewFileBase(filename), input, p.error, p.pragma, syntax.CheckBranches)
	}()
	if len(failures) != 0 {
		return nil, failures
	}
	if p.file == nil {
		return nil, fmt.Errorf("noder parser returned no syntax tree")
	}
	return &ParsedFile{native: p}, nil
}

// WriteParsedPackage preserves native linkname/cgo/pragma metadata while
// writing successfully checked and prepared trees. The checker, import loading
// and range-function rewrite are still explicit caller phases at this entry.
func (c *Context) WriteParsedPackage(pkg *types2.Package, info *types2.Info, parsed []*ParsedFile, rangeClosures map[*syntax.FuncLit]bool) (string, error) {
	noders := make([]*noder, len(parsed))
	files := make([]*syntax.File, len(parsed))
	for i, f := range parsed {
		if f == nil || f.native == nil || f.native.compilation != c || f.native.file == nil {
			return "", fmt.Errorf("parsed source belongs to another or uninitialized compilation")
		}
		noders[i], files[i] = f.native, f.native.file
	}
	return c.writeCheckedPackage(pkg, info, files, noders, rangeClosures)
}

// DependencyFile is a caller-supplied archive handle. Native import processing
// closes each successfully opened handle after reading its retained export data.
type DependencyFile interface {
	io.Reader
	io.ReaderAt
	io.Seeker
	io.Closer
	Name() string
}

// ObjectConfig contains the target features and version used by native object
// compatibility checks. Experiment flags come from this compiler's Universe.
type ObjectConfig = buildcfg.Context

// BindImports configures native package lookup and archive validation before
// the first dependency or local package is loaded. HostOS selects path syntax;
// it is independent of the compilation target. Lookup flags live in Base.Flag.
func (c *Context) BindImports(config ObjectConfig, hostOS string, open func(string) (DependencyFile, error)) {
	if c == nil || c.Typecheck == nil || c.Checker == nil || c.importHeader != nil || c.localPkgReader != nil || len(c.Typecheck.Target.Imports) != 0 || config.GOARCH != c.GOARCH || config.GOOS == "" || config.GOOS != c.Ctxt.TargetOS || config.Version == "" || hostOS == "" || open == nil || c.GOOS != "" && c.GOOS != config.GOOS {
		panic("dependency inputs require a fresh matching compiler context")
	}
	c.GOOS = config.GOOS
	c.hostOS = hostOS
	c.importHeader = &objabi.Context{ObjectConfig: &config, Experiment: c.Types.Experiment}
	c.dependencyOpen = open
	c.importChecker = c.Checker
	c.typeImports = importer.NewContext(c.Checker)
	c.importPackages = make(map[string]*types2.Package)
}
func (c *Context) openDependency(name string) (DependencyFile, error) {
	if c.dependencyOpen == nil {
		panic("dependency stream provider is unbound")
	}
	f, err := c.dependencyOpen(name)
	if f == nil && err == nil {
		return nil, fmt.Errorf("dependency provider returned a nil stream for %q", name)
	}
	return f, err
}

// Import runs the native compiler and type-checker dependency readers together.
// Calls and lazy declarations within a compilation are serial. A failed import
// can leave native direct-import state partially filled; discard this context.
func (c *Context) Import(path string) (pkg *types2.Package, err error) {
	if c == nil || c.importHeader == nil || c.Checker != c.importChecker || c.importFailed || c.localPkgReader != nil {
		return nil, fmt.Errorf("dependency import requires an unfailed bound compiler context")
	}
	c.importStarted = true
	completed := false
	defer func() {
		if !completed || err != nil {
			c.importFailed = true
		}
	}()
	_, pkg, err = c.readImportFile(path, c.Typecheck.Target, c.Checker, c.importPackages)
	completed = true
	return
}

func (c *Context) openSyntaxHTML(path string, flags int, mode fs.FileMode) (io.WriteCloser, error) {
	if c.OpenSyntaxHTML == nil {
		return nil, fmt.Errorf("AST HTML output provider is not configured")
	}
	w, e := c.OpenSyntaxHTML(path, flags, mode)
	if e == nil && w == nil {
		return nil, fmt.Errorf("AST HTML output provider returned nil writer")
	}
	return w, e
}
func (c *Context) syntaxWorkingDirectory() (string, error) {
	if c.SyntaxWorkingDirectory == nil {
		return "", fmt.Errorf("AST HTML working directory provider is not configured")
	}
	return c.SyntaxWorkingDirectory()
}

// SourceErrors retains a detached diagnostic snapshot from native source
// checking or unified construction. Internal and provider panics are distinct.
type SourceErrors []diagnostic.Diagnostic

func (e SourceErrors) Error() string {
	for _, d := range e {
		if d.Severity == diagnostic.Error {
			return "noder: " + d.String()
		}
	}
	return "noder: source compilation failed"
}
func (c *Context) syntaxErrors() int {
	n := 0
	for _, d := range c.Diagnostics.Diagnostics() {
		if d.Severity == diagnostic.Error && strings.HasPrefix(d.Message, "syntax error") {
			n++
		}
	}
	return n
}
func (c *Context) exitIfErrors() {
	if c.Diagnostics.HasErrors() {
		c.errorExit()
	}
}
func (c *Context) freshChecker() *types2.Context {
	checker := types2.NewContext()
	c.Checker, c.importChecker = checker, checker
	c.typeImports = importer.NewContext(checker)
	c.importPackages = make(map[string]*types2.Package)
	c.initializeWriterUniverse()
	return checker
}
func (c *Context) rewriteMidway(pkg *types2.Package, info *types2.Info, files []*syntax.File) bool {
	m := midway.NewContext(c.Checker, c.GOARCH, c.Types.Experiment, *c.Debug)
	m.Warn = c.warn
	return m.RewriteWrapper(pkg, info, files)
}
func (c *Context) rewriteRangeFuncs(pkg *types2.Package, info *types2.Info, files []*syntax.File) map[*syntax.FuncLit]bool {
	r := rangefunc.NewContext(c.Checker)
	r.Flag, r.Debug, r.TraceWriter = *c.Flag, *c.Debug, c.traceWriter()
	return r.Rewrite(pkg, info, files)
}

// CompileParsedPackage runs the full native checkFiles and unified frontend,
// including SIMD rechecking, range-function rewriting and reader validation.
// Configure native dependency inputs, runtime, reflection and static-init first.
// This consumes a fresh compilation once. Each retained ParsedFile must belong
// to this Context; callers must not concurrently mutate its trees or inputs.
func (c *Context) CompileParsedPackage(parsed []*ParsedFile) (target *ir.Package, err error) {
	if c == nil || c.sourceStarted || c.importHeader == nil || c.Checker != c.importChecker || c.importStarted || c.importFailed || c.localPkgReader != nil || c.checked != nil || c.Diagnostics.HasErrors() || len(parsed) == 0 {
		return nil, fmt.Errorf("source compilation requires a fresh bound compiler and parsed files")
	}
	if c.ReflectData == nil || c.ReflectData.Typecheck != c.Typecheck || c.StaticInit == nil || c.StaticInit.Typecheck != c.Typecheck || c.StaticInit.ReflectData != c.ReflectData || len(c.Typecheck.Target.Funcs) != 0 || len(c.Typecheck.Target.Imports) != 0 {
		return nil, fmt.Errorf("source compilation requires a fresh target and shared reflection/static initialization")
	}
	if c.Debug.AstDump != "" && (c.OpenSyntaxHTML == nil || c.SyntaxWorkingDirectory == nil) {
		return nil, fmt.Errorf("AST diagnostics require output and working directory providers")
	}
	noders := make([]*noder, len(parsed))
	seen := make(map[*noder]bool)
	for i, f := range parsed {
		if f == nil || f.native == nil || f.native.compilation != c || f.native.file == nil || seen[f.native] {
			return nil, fmt.Errorf("source file is nil, duplicated or belongs to another compilation")
		}
		seen[f.native] = true
		noders[i] = f.native
	}
	c.sourceStarted, c.sourceMode = true, true
	defer func() {
		failure := recover()
		func() {
			defer func() {
				if p := recover(); failure == nil {
					failure = p
				}
			}()
			c.CloseHTMLWriters()
		}()
		c.sourceMode = false
		c.checked = nil
		c.info = nil
		c.rangeClosures = nil
		if failure != nil {
			if diagnostics, ok := failure.(SourceErrors); ok {
				target = nil
				err = diagnostics
			} else {
				panic(failure)
			}
		}
	}()
	c.unified(posMap{compilation: c}, noders)
	return c.Typecheck.Target, nil
}

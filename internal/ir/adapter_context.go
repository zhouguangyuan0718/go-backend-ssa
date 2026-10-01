package ir

import (
	"encoding/base64"
	"fmt"
	"go-backend-ssa/diagnostic"
	"go-backend-ssa/internal/base"
	"go-backend-ssa/internal/cmd/obj"
	"go-backend-ssa/internal/cmd/src"
	errors "go-backend-ssa/internal/go/types/errors"
	"go-backend-ssa/internal/types"
	"go/constant"
	"io"
	"io/fs"
	"os"
	"reflect"
	"regexp"
	"strings"
	"sync"
)

// Context owns native IR state. A context and its graph are used serially;
// independent contexts share no mutable compiler state.
type Context struct {
	astAppendFlag, astReadWriteFlag, astCreateFlag, astTruncateFlag int
	// OpenAST supplies native text and HTML report streams. The caller honors
	// the passed append/create/truncate flags and owns the output destination.
	OpenAST             func(string, int, fs.FileMode) (io.WriteCloser, error)
	ASTWorkingDirectory func() (string, error)
	astMu               sync.Mutex
	astDumpFiles        map[string]bool
	astHTMLWriters      map[*Func]*HTMLWriter
	astOrderedFuncs     []*Func
	dumpIndentBytes     []byte

	safePhaseNameString *regexp.Regexp
	Syms                symsStruct
	Pkgs                struct{ Go, Itab, Runtime, InternalMaps, Coverage *types.Pkg }

	Types                                                         *types.Universe
	Base                                                          *base.LinkContext
	Debug                                                         base.DebugFlags
	CurFunc                                                       *Func
	BlankNode                                                     *Name
	globClosgen                                                   int32
	IsIntrinsicCall                                               func(*CallExpr) bool
	IsIntrinsicSym                                                func(*types.Sym) bool
	EscFmt                                                        func(n Node) string
	nodeType                                                      reflect.Type
	intZero, floatZero, complexZero, intOne, floatOne, complexOne constant.Value
	OKForConst                                                    [types.NTYPE]bool
	MaxStackVarSize, MaxImplicitStackVarSize, MaxSmallArraySize   int64
	TraceWriter                                                   io.Writer
	Diagnostics                                                   diagnostic.Collector
}

func NewContext(u *types.Universe, link *obj.Link) *Context {
	if u == nil || link == nil {
		panic("IR requires compiler types and Link")
	}
	if u.PosTable != &link.PosTable {
		panic("IR and compiler types require the same position table from construction")
	}
	if link.Arch != nil && link.Arch.Arch != nil && (link.Arch.PtrSize != u.PointerSize() || link.Arch.RegSize != u.RegisterSize()) {
		panic("IR target layout mismatch")
	}
	u.BindLink(link)
	c := &Context{Types: u, Base: u.LinkContext(), astAppendFlag: os.O_APPEND, astReadWriteFlag: os.O_RDWR, astCreateFlag: os.O_CREATE, astTruncateFlag: os.O_TRUNC}
	c.initialize()
	// Native driver pseudo-packages from gc/main.go. Keep their source identity
	// separate from real imported packages while using the original link prefixes.
	c.Pkgs.Runtime = u.NewPkg("go.runtime", "runtime")
	c.Pkgs.Runtime.Prefix = "runtime"
	c.Pkgs.InternalMaps = u.NewPkg("go.internal/runtime/maps", "internal/runtime/maps")
	c.Pkgs.InternalMaps.Prefix = "internal/runtime/maps"
	c.Pkgs.Itab = u.NewPkg("go.itab", "go.itab")
	c.Pkgs.Itab.Prefix = "go:itab"
	c.Pkgs.Go = u.NewPkg("go", "")
	c.Pkgs.Coverage = u.NewPkg("go.coverage", "runtime/coverage")
	c.Pkgs.Coverage.Prefix = "runtime/coverage"
	return c
}

func (n *miniNode) compilationContext() *Context { return n.compilation }

// BindCaseClause attaches walk's raw native case literal without changing its
// original zero position/op or child slices. Foreign children are rejected
// before the node gains an owner.
func (c *Context) BindCaseClause(n *CaseClause) *CaseClause {
	if c == nil || n == nil || n.compilation != nil {
		panic("case binding requires an unbound literal")
	}
	if n.Type() != nil && n.Type().Universe() != c.Types {
		panic("case type belongs to another compilation")
	}
	if n.Var != nil && !c.OwnsNode(n.Var) {
		panic("case variable belongs to another compilation")
	}
	for _, list := range []Nodes{n.Init(), n.List, n.RTypes, n.Body} {
		for _, child := range list {
			if child != nil && !c.OwnsNode(child) {
				panic("case child belongs to another compilation")
			}
		}
	}
	n.compilation = c
	return n
}
func nodeContext(n Node) *Context {
	if n == nil {
		c := new(Context)
		c.initialize()
		return c
	}
	c := n.(interface{ compilationContext() *Context }).compilationContext()
	if c == nil {
		panic("IR node has no compiler context")
	}
	return c
}
func nodesContext(nodes Nodes) *Context {
	var c *Context
	for _, n := range nodes {
		if n != nil {
			owner := nodeContext(n)
			if c != nil && c != owner {
				panic("mixed IR contexts")
			}
			c = owner
		}
	}
	if c == nil {
		return nodeContext(nil)
	}
	return c
}

// The Op predicate has no state; only its invalid-op diagnostic needs a receiver.
func opContext(Op) *Context { return &Context{} }

func (c *Context) fatalf(format string, args ...any) { panic(fmt.Errorf(format, args...)) }
func (c *Context) fatalfAt(pos src.XPos, format string, args ...any) {
	panic(fmt.Errorf("%s: %s", c.Base.FmtPos(pos), fmt.Sprintf(format, args...)))
}
func (c *Context) errorfAt(pos src.XPos, code errors.Code, format string, args ...any) {
	c.Diagnostics.Add(diagnostic.Error, code.String(), c.Base.Ctxt.PosTable.Pos(pos), fmt.Sprintf(format, args...))
	c.Base.ReportError(pos, code, format, args...)
}
func (c *Context) warnfAt(pos src.XPos, format string, args ...any) {
	c.Diagnostics.Add(diagnostic.Warning, "", c.Base.Ctxt.PosTable.Pos(pos), fmt.Sprintf(format, args...))
	c.Base.ReportWarning(pos, format, args...)
}
func (c *Context) warn(format string, args ...any) { c.warnfAt(c.Types.Pos, format, args...) }
func (c *Context) traceWriter() io.Writer {
	if c.TraceWriter != nil {
		return c.TraceWriter
	}
	return io.Discard
}
func (c *Context) tracePrintf(format string, args ...any) (int, error) {
	return fmt.Fprintf(c.traceWriter(), format, args...)
}

// Native obj.TrimInlineHash algorithm, with the immutable package initializer
// localized. Keep in sync with src/cmd/internal/obj/objfile.go at the pinned Go.
func (c *Context) trimInlineHash(name string) string {
	inlHashLen := base64.StdEncoding.EncodedLen(8)
	a, b, ok := strings.Cut(name, "#")
	if !ok {
		return name
	}
	if len(b) < inlHashLen+1 || b[inlHashLen] != '#' {
		return name
	}
	return a + b[inlHashLen+1:]
}

// The native oracle already retains its analyzed function. Use that function's
// graph for diagnostics and helper calls, keeping the upstream zero-value Init.
func oracleContext(ro *ReassignOracle) *Context {
	if ro == nil || ro.fn == nil {
		panic("reassignment oracle is not initialized")
	}
	return nodeContext(ro.fn)
}

func (c *Context) checkOracleNode(n Node) {
	if n == nil || nodeContext(n) != c {
		panic("oracle node belongs to another compilation")
	}
}

// OwnsNode checks the existing native node's compilation owner.
func (c *Context) OwnsNode(n Node) bool {
	if n == nil {
		return false
	}
	owner, ok := n.(interface{ compilationContext() *Context })
	return ok && owner.compilationContext() == c
}

// Validate the graph before native symbol initialization mutates either owner.
func (c *Context) checkInitLSym(f *Func, hasBody bool) {
	if !c.OwnsNode(f) {
		panic("function symbol belongs to another compilation")
	}
}

// ContextOf returns the existing owner of an initialized native IR node for
// compiler packages that derive their policy from a node. It creates no owner.
func ContextOf(n Node) *Context {
	if n == nil {
		panic("IR node has no compiler context")
	}
	return nodeContext(n)
}

// htmlBaseContext follows the buffer installed by native HTMLWriterBase.Init.
// The buffer is allocated by this compilation; no process-wide writer registry
// or synthetic compilation is needed for promoted HTMLWriterBase methods.
func htmlBaseContext(w *HTMLWriterBase) *Context {
	if w == nil || w.w == nil || w.w.compilation == nil {
		panic("HTML writer has no compilation owner")
	}
	return w.w.compilation
}

func (c *Context) openAST(path string, flags int, mode fs.FileMode) (io.WriteCloser, error) {
	if c.OpenAST == nil {
		return nil, fmt.Errorf("IR AST output provider is not configured")
	}
	w, e := c.OpenAST(path, flags, mode)
	if e == nil && w == nil {
		return nil, fmt.Errorf("IR AST output provider returned nil writer")
	}
	return w, e
}
func (c *Context) astWorkingDirectory() (string, error) {
	if c.ASTWorkingDirectory == nil {
		return "", fmt.Errorf("IR AST working directory provider is not configured")
	}
	return c.ASTWorkingDirectory()
}
func (c *Context) checkAstDump(fn *Func, why string) {
	if fn == nil || !c.OwnsNode(fn) {
		panic("AST dump belongs to another compilation")
	}
}
func (c *Context) checkDumpNodeHTML(fn *Func, why string, n Node) {
	c.checkAstDump(fn, why)
	if n == nil || !c.OwnsNode(n) {
		panic("HTML dump node belongs to another compilation")
	}
}
func (c *Context) checkHTMLWriter(path string, fn *Func, cfgMask string) { c.checkAstDump(fn, "") }
func (c *Context) checkDumpAny(w io.Writer, root any, filter string, depth int) {
	if n, ok := root.(Node); ok {
		// Native FDumpAny handles typed nil pointers before dereferencing them.
		v := reflect.ValueOf(n)
		if v.Kind() == reflect.Pointer && v.IsNil() {
			return
		}
		if !c.OwnsNode(n) {
			panic("generic AST dump belongs to another compilation")
		}
	}
}

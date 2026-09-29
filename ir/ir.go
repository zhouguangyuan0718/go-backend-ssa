// Package ir exposes native Go compiler IR nodes and instance-owned constructors.
// Nodes are aliases, so no graph translation is needed at the backend boundary.
package ir

import (
	"go-backend-ssa/gcbridge"
	"go-backend-ssa/internal/cmd/src"
	compiler "go-backend-ssa/internal/ir"
	"go-backend-ssa/types"
	"go/constant"
	"io"
)

type Context = compiler.Context

// NewContext binds types and IR to the same native Link. Construct the Universe
// with types.NewUniverseWithPositions(layout, &link.PosTable). All inputs and
// mutable nodes must belong to that instance; node fields remain native and
// directly editable. This does not initialize the typecheck universe.
func NewContext(u *types.Universe, link *gcbridge.Link) *Context { return compiler.NewContext(u, link) }

type Node = compiler.Node

func IsSynthetic(n Node) bool { return compiler.IsSynthetic(n) }
func IsAutoTmp(n Node) bool   { return compiler.IsAutoTmp(n) }
func MayBeShared(n Node) bool { return compiler.MayBeShared(n) }

type InitNode = compiler.InitNode

func TakeInit(n Node) Nodes { return compiler.TakeInit(n) }

type Op = compiler.Op

const OXXX = compiler.OXXX
const ONAME = compiler.ONAME
const ONONAME = compiler.ONONAME
const OTYPE = compiler.OTYPE
const OLITERAL = compiler.OLITERAL
const ONIL = compiler.ONIL
const OADD = compiler.OADD
const OSUB = compiler.OSUB
const OOR = compiler.OOR
const OXOR = compiler.OXOR
const OADDSTR = compiler.OADDSTR
const OADDR = compiler.OADDR
const OANDAND = compiler.OANDAND
const OAPPEND = compiler.OAPPEND
const OBYTES2STR = compiler.OBYTES2STR
const OBYTES2STRTMP = compiler.OBYTES2STRTMP
const ORUNES2STR = compiler.ORUNES2STR
const OSTR2BYTES = compiler.OSTR2BYTES
const OSTR2BYTESTMP = compiler.OSTR2BYTESTMP
const OSTR2RUNES = compiler.OSTR2RUNES
const OSLICE2ARR = compiler.OSLICE2ARR
const OSLICE2ARRPTR = compiler.OSLICE2ARRPTR
const OAS = compiler.OAS
const OAS2 = compiler.OAS2
const OAS2DOTTYPE = compiler.OAS2DOTTYPE
const OAS2FUNC = compiler.OAS2FUNC
const OAS2MAPR = compiler.OAS2MAPR
const OAS2RECV = compiler.OAS2RECV
const OASOP = compiler.OASOP
const OCALL = compiler.OCALL
const OCALLFUNC = compiler.OCALLFUNC
const OCALLMETH = compiler.OCALLMETH
const OCALLINTER = compiler.OCALLINTER
const OCAP = compiler.OCAP
const OCLEAR = compiler.OCLEAR
const OCLOSE = compiler.OCLOSE
const OCLOSURE = compiler.OCLOSURE
const OCOMPLIT = compiler.OCOMPLIT
const OMAPLIT = compiler.OMAPLIT
const OSTRUCTLIT = compiler.OSTRUCTLIT
const OARRAYLIT = compiler.OARRAYLIT
const OSLICELIT = compiler.OSLICELIT
const OPTRLIT = compiler.OPTRLIT
const OCONV = compiler.OCONV
const OCONVIFACE = compiler.OCONVIFACE
const OCONVNOP = compiler.OCONVNOP
const OCOPY = compiler.OCOPY
const ODCL = compiler.ODCL
const ODCLFUNC = compiler.ODCLFUNC
const ODELETE = compiler.ODELETE
const ODOT = compiler.ODOT
const ODOTPTR = compiler.ODOTPTR
const ODOTMETH = compiler.ODOTMETH
const ODOTINTER = compiler.ODOTINTER
const OXDOT = compiler.OXDOT
const ODOTTYPE = compiler.ODOTTYPE
const ODOTTYPE2 = compiler.ODOTTYPE2
const OEQ = compiler.OEQ
const ONE = compiler.ONE
const OLT = compiler.OLT
const OLE = compiler.OLE
const OGE = compiler.OGE
const OGT = compiler.OGT
const ODEREF = compiler.ODEREF
const OINDEX = compiler.OINDEX
const OINDEXMAP = compiler.OINDEXMAP
const OKEY = compiler.OKEY
const OSTRUCTKEY = compiler.OSTRUCTKEY
const OLEN = compiler.OLEN
const OMAKE = compiler.OMAKE
const OMAKECHAN = compiler.OMAKECHAN
const OMAKEMAP = compiler.OMAKEMAP
const OMAKESLICE = compiler.OMAKESLICE
const OMAKESLICECOPY = compiler.OMAKESLICECOPY
const OMUL = compiler.OMUL
const ODIV = compiler.ODIV
const OMOD = compiler.OMOD
const OLSH = compiler.OLSH
const ORSH = compiler.ORSH
const OAND = compiler.OAND
const OANDNOT = compiler.OANDNOT
const ONEW = compiler.ONEW
const ONOT = compiler.ONOT
const OBITNOT = compiler.OBITNOT
const OPLUS = compiler.OPLUS
const ONEG = compiler.ONEG
const OOROR = compiler.OOROR
const OPANIC = compiler.OPANIC
const OPRINT = compiler.OPRINT
const OPRINTLN = compiler.OPRINTLN
const OPAREN = compiler.OPAREN
const OSEND = compiler.OSEND
const OSLICE = compiler.OSLICE
const OSLICEARR = compiler.OSLICEARR
const OSLICESTR = compiler.OSLICESTR
const OSLICE3 = compiler.OSLICE3
const OSLICE3ARR = compiler.OSLICE3ARR
const OSLICEHEADER = compiler.OSLICEHEADER
const OSTRINGHEADER = compiler.OSTRINGHEADER
const ORECOVER = compiler.ORECOVER
const ORECV = compiler.ORECV
const ORUNESTR = compiler.ORUNESTR
const OSELRECV2 = compiler.OSELRECV2
const OMIN = compiler.OMIN
const OMAX = compiler.OMAX
const OREAL = compiler.OREAL
const OIMAG = compiler.OIMAG
const OCOMPLEX = compiler.OCOMPLEX
const OUNSAFEADD = compiler.OUNSAFEADD
const OUNSAFESLICE = compiler.OUNSAFESLICE
const OUNSAFESLICEDATA = compiler.OUNSAFESLICEDATA
const OUNSAFESTRING = compiler.OUNSAFESTRING
const OUNSAFESTRINGDATA = compiler.OUNSAFESTRINGDATA
const OMETHEXPR = compiler.OMETHEXPR
const OMETHVALUE = compiler.OMETHVALUE
const OBLOCK = compiler.OBLOCK
const OBREAK = compiler.OBREAK
const OCASE = compiler.OCASE
const OCONTINUE = compiler.OCONTINUE
const ODEFER = compiler.ODEFER
const OFALL = compiler.OFALL
const OFOR = compiler.OFOR
const OGOTO = compiler.OGOTO
const OIF = compiler.OIF
const OLABEL = compiler.OLABEL
const OGO = compiler.OGO
const ORANGE = compiler.ORANGE
const ORETURN = compiler.ORETURN
const OSELECT = compiler.OSELECT
const OSWITCH = compiler.OSWITCH
const OTYPESW = compiler.OTYPESW
const OINLCALL = compiler.OINLCALL
const OMAKEFACE = compiler.OMAKEFACE
const OITAB = compiler.OITAB
const OIDATA = compiler.OIDATA
const OSPTR = compiler.OSPTR
const OCFUNC = compiler.OCFUNC
const OCHECKNIL = compiler.OCHECKNIL
const ORESULT = compiler.ORESULT
const OINLMARK = compiler.OINLMARK
const OLINKSYMOFFSET = compiler.OLINKSYMOFFSET
const OJUMPTABLE = compiler.OJUMPTABLE
const OINTERFACESWITCH = compiler.OINTERFACESWITCH
const OMOVE2HEAP = compiler.OMOVE2HEAP
const ODYNAMICDOTTYPE = compiler.ODYNAMICDOTTYPE
const ODYNAMICDOTTYPE2 = compiler.ODYNAMICDOTTYPE2
const ODYNAMICTYPE = compiler.ODYNAMICTYPE
const OTAILCALL = compiler.OTAILCALL
const OGETG = compiler.OGETG
const OGETCALLERSP = compiler.OGETCALLERSP
const OEND = compiler.OEND

type Nodes = compiler.Nodes

func ToNodes[T Node](s []T) Nodes { return compiler.ToNodes(s) }

type NameQueue = compiler.NameQueue
type NameSet = compiler.NameSet
type PragmaFlag = compiler.PragmaFlag

const Nointerface = compiler.Nointerface
const Noescape = compiler.Noescape
const Norace = compiler.Norace
const Nosplit = compiler.Nosplit
const Noinline = compiler.Noinline
const NoCheckPtr = compiler.NoCheckPtr
const CgoUnsafeArgs = compiler.CgoUnsafeArgs
const UintptrKeepAlive = compiler.UintptrKeepAlive
const UintptrEscapes = compiler.UintptrEscapes
const Systemstack = compiler.Systemstack
const Nowritebarrier = compiler.Nowritebarrier
const Nowritebarrierrec = compiler.Nowritebarrierrec
const Yeswritebarrierrec = compiler.Yeswritebarrierrec
const GoBuildPragma = compiler.GoBuildPragma
const RegisterParams = compiler.RegisterParams

func IsConst(n Node, ct constant.Kind) bool { return compiler.IsConst(n, ct) }
func IsNil(n Node) bool                     { return compiler.IsNil(n) }
func IsBlank(n Node) bool                   { return compiler.IsBlank(n) }
func IsMethod(n Node) bool                  { return compiler.IsMethod(n) }

const EscUnknown = compiler.EscUnknown
const EscNone = compiler.EscNone
const EscHeap = compiler.EscHeap
const EscNever = compiler.EscNever

type Expr = compiler.Expr
type AddStringExpr = compiler.AddStringExpr
type AddrExpr = compiler.AddrExpr
type BasicLit = compiler.BasicLit
type BinaryExpr = compiler.BinaryExpr
type CallExpr = compiler.CallExpr
type ClosureExpr = compiler.ClosureExpr
type CompLitExpr = compiler.CompLitExpr
type ConvExpr = compiler.ConvExpr
type IndexExpr = compiler.IndexExpr
type KeyExpr = compiler.KeyExpr
type StructKeyExpr = compiler.StructKeyExpr
type InlinedCallExpr = compiler.InlinedCallExpr
type LogicalExpr = compiler.LogicalExpr
type MakeExpr = compiler.MakeExpr
type NilExpr = compiler.NilExpr
type ParenExpr = compiler.ParenExpr
type ResultExpr = compiler.ResultExpr
type LinksymOffsetExpr = compiler.LinksymOffsetExpr
type SelectorExpr = compiler.SelectorExpr
type SliceExpr = compiler.SliceExpr
type SliceHeaderExpr = compiler.SliceHeaderExpr
type StringHeaderExpr = compiler.StringHeaderExpr
type StarExpr = compiler.StarExpr
type TypeAssertExpr = compiler.TypeAssertExpr
type DynamicTypeAssertExpr = compiler.DynamicTypeAssertExpr
type UnaryExpr = compiler.UnaryExpr

func IsZero(n Node) bool                   { return compiler.IsZero(n) }
func IsAddressable(n Node) bool            { return compiler.IsAddressable(n) }
func ParamNames(ft *types.Type) []Node     { return compiler.ParamNames(ft) }
func RecvParamNames(ft *types.Type) []Node { return compiler.RecvParamNames(ft) }
func LookupMethodSelector(pkg *types.Pkg, name string) (typ, meth *types.Sym, err error) {
	return compiler.LookupMethodSelector(pkg, name)
}

type MoveToHeapExpr = compiler.MoveToHeapExpr
type Decl = compiler.Decl
type Stmt = compiler.Stmt
type AssignListStmt = compiler.AssignListStmt
type AssignStmt = compiler.AssignStmt
type AssignOpStmt = compiler.AssignOpStmt
type BlockStmt = compiler.BlockStmt
type BranchStmt = compiler.BranchStmt
type CaseClause = compiler.CaseClause
type CommClause = compiler.CommClause
type ForStmt = compiler.ForStmt
type GoDeferStmt = compiler.GoDeferStmt
type IfStmt = compiler.IfStmt
type JumpTableStmt = compiler.JumpTableStmt
type InterfaceSwitchStmt = compiler.InterfaceSwitchStmt
type InlineMarkStmt = compiler.InlineMarkStmt
type LabelStmt = compiler.LabelStmt
type RangeStmt = compiler.RangeStmt
type ReturnStmt = compiler.ReturnStmt
type SelectStmt = compiler.SelectStmt
type SendStmt = compiler.SendStmt
type SwitchStmt = compiler.SwitchStmt
type TailCallStmt = compiler.TailCallStmt
type TypeSwitchGuard = compiler.TypeSwitchGuard
type Ident = compiler.Ident
type Name = compiler.Name

func SameSource(n1, n2 Node) bool { return compiler.SameSource(n1, n2) }

type Class = compiler.Class

const Pxxx = compiler.Pxxx
const PEXTERN = compiler.PEXTERN
const PAUTO = compiler.PAUTO
const PAUTOHEAP = compiler.PAUTOHEAP
const PPARAM = compiler.PPARAM
const PPARAMOUT = compiler.PPARAMOUT
const PTYPEPARAM = compiler.PTYPEPARAM
const PFUNC = compiler.PFUNC

type Embed = compiler.Embed
type Func = compiler.Func
type WasmImport = compiler.WasmImport
type WasmExport = compiler.WasmExport
type Inline = compiler.Inline
type Mark = compiler.Mark
type ScopeID = compiler.ScopeID
type SymAndPos = compiler.SymAndPos

func FuncName(f *Func) string     { return compiler.FuncName(f) }
func PkgFuncName(f *Func) string  { return compiler.PkgFuncName(f) }
func LinkFuncName(f *Func) string { return compiler.LinkFuncName(f) }
func ParseLinkFuncName(name string) (pkg, sym string, err error) {
	return compiler.ParseLinkFuncName(name)
}
func FuncSymName(s *types.Sym) string    { return compiler.FuncSymName(s) }
func IsFuncPCIntrinsic(n *CallExpr) bool { return compiler.IsFuncPCIntrinsic(n) }
func IsIfaceOfFunc(n Node) *Func         { return compiler.IsIfaceOfFunc(n) }
func ContainsClosure(f, c *Func) bool    { return compiler.ContainsClosure(f, c) }

type DynamicType = compiler.DynamicType
type Package = compiler.Package

func DoChildren(n Node, do func(Node) bool) bool { return compiler.DoChildren(n, do) }
func DoChildrenWithHidden(n Node, do func(Node) bool) bool {
	return compiler.DoChildrenWithHidden(n, do)
}
func Visit(n Node, visit func(Node))                      { compiler.Visit(n, visit) }
func VisitList(list Nodes, visit func(Node))              { compiler.VisitList(list, visit) }
func VisitFuncAndClosures(fn *Func, visit func(n Node))   { compiler.VisitFuncAndClosures(fn, visit) }
func Any(n Node, cond func(Node) bool) bool               { return compiler.Any(n, cond) }
func EditChildren(n Node, edit func(Node) Node)           { compiler.EditChildren(n, edit) }
func EditChildrenWithHidden(n Node, edit func(Node) Node) { compiler.EditChildrenWithHidden(n, edit) }
func Copy(n Node) Node                                    { return compiler.Copy(n) }
func DeepCopy(pos src.XPos, n Node) Node                  { return compiler.DeepCopy(pos, n) }
func StmtWithInit(op Op) bool                             { return compiler.StmtWithInit(op) }
func FDump(w io.Writer, s string, n Node)                 { compiler.FDump(w, s, n) }

const ConstPrec = compiler.ConstPrec

func IsConstNode(n Node) bool        { return compiler.IsConstNode(n) }
func IsSmallIntConst(n Node) bool    { return compiler.IsSmallIntConst(n) }
func ConstType(n Node) constant.Kind { return compiler.ConstType(n) }

// ReassignOracle is the native function-local reassignment analysis.
type ReassignOracle = compiler.ReassignOracle

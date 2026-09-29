// Package types2 exposes the Go 1.27.1 compiler's source type checker.
//
// Create a Context and set Config.Context explicitly. Packages and imports in
// one checking operation must belong to the same Context. Independent contexts
// have separate predeclared types, scopes, caches, and diagnostic output.
// For a possibly nil package, use Context.PackageScope: nil selects that
// instance's Universe. Package.Scope itself requires a nonnil owner.
// Set Config.Sizes for the target and Config.GoVersion for the source language.
// A nil Context.TraceWriter discards tracing; no process output is used.
// Callers must not mutate shared ASTs, options or result graphs during a check.
package types2

import (
	impl "go-backend-ssa/internal/types2"
	"go-backend-ssa/syntax"
)

type (
	Alias         = impl.Alias
	ArgumentError = impl.ArgumentError
	Array         = impl.Array
	Basic         = impl.Basic
	BasicInfo     = impl.BasicInfo
	BasicKind     = impl.BasicKind
	Builtin       = impl.Builtin
	Chan          = impl.Chan
	ChanDir       = impl.ChanDir
	Checker       = impl.Checker
	Config        = impl.Config
	Const         = impl.Const
	Context       = impl.Context
	Error         = impl.Error
	Func          = impl.Func
	ImportMode    = impl.ImportMode
	Importer      = impl.Importer
	ImporterFrom  = impl.ImporterFrom
	Info          = impl.Info
	Initializer   = impl.Initializer
	Instance      = impl.Instance
	Interface     = impl.Interface
	Label         = impl.Label
	Map           = impl.Map
	Named         = impl.Named
	Nil           = impl.Nil
	Object        = impl.Object
	Package       = impl.Package
	PkgName       = impl.PkgName
	Pointer       = impl.Pointer
	Qualifier     = impl.Qualifier
	Scope         = impl.Scope
	Selection     = impl.Selection
	SelectionKind = impl.SelectionKind
	Signature     = impl.Signature
	Sizes         = impl.Sizes
	Slice         = impl.Slice
	StdSizes      = impl.StdSizes
	Struct        = impl.Struct
	Term          = impl.Term
	Tuple         = impl.Tuple
	Type          = impl.Type
	TypeAndValue  = impl.TypeAndValue
	TypeList      = impl.TypeList
	TypeName      = impl.TypeName
	TypeParam     = impl.TypeParam
	TypeParamList = impl.TypeParamList
	Union         = impl.Union
	Var           = impl.Var
	VarKind       = impl.VarKind
)

const (
	Bool           = impl.Bool
	Byte           = impl.Byte
	Complex128     = impl.Complex128
	Complex64      = impl.Complex64
	FieldVal       = impl.FieldVal
	FieldVar       = impl.FieldVar
	Float32        = impl.Float32
	Float64        = impl.Float64
	Int            = impl.Int
	Int16          = impl.Int16
	Int32          = impl.Int32
	Int64          = impl.Int64
	Int8           = impl.Int8
	Invalid        = impl.Invalid
	IsBoolean      = impl.IsBoolean
	IsComplex      = impl.IsComplex
	IsConstType    = impl.IsConstType
	IsFloat        = impl.IsFloat
	IsInteger      = impl.IsInteger
	IsNumeric      = impl.IsNumeric
	IsOrdered      = impl.IsOrdered
	IsString       = impl.IsString
	IsUnsigned     = impl.IsUnsigned
	IsUntyped      = impl.IsUntyped
	LocalVar       = impl.LocalVar
	MethodExpr     = impl.MethodExpr
	MethodVal      = impl.MethodVal
	PackageVar     = impl.PackageVar
	ParamVar       = impl.ParamVar
	RecvOnly       = impl.RecvOnly
	RecvVar        = impl.RecvVar
	ResultVar      = impl.ResultVar
	Rune           = impl.Rune
	SendOnly       = impl.SendOnly
	SendRecv       = impl.SendRecv
	String         = impl.String
	Uint           = impl.Uint
	Uint16         = impl.Uint16
	Uint32         = impl.Uint32
	Uint64         = impl.Uint64
	Uint8          = impl.Uint8
	Uintptr        = impl.Uintptr
	UnsafePointer  = impl.UnsafePointer
	UntypedBool    = impl.UntypedBool
	UntypedComplex = impl.UntypedComplex
	UntypedFloat   = impl.UntypedFloat
	UntypedInt     = impl.UntypedInt
	UntypedNil     = impl.UntypedNil
	UntypedRune    = impl.UntypedRune
	UntypedString  = impl.UntypedString
)

func NewContext() *Context                { return impl.NewContext() }
func Unalias(t Type) Type                 { return impl.Unalias(t) }
func AsPointer(t Type) *Pointer           { return impl.AsPointer(t) }
func IsInterface(t Type) bool             { return impl.IsInterface(t) }
func Comparable(t Type) bool              { return impl.Comparable(t) }
func Id(pkg *Package, name string) string { return impl.Id(pkg, name) }
func ExprString(x syntax.Node) string     { return impl.ExprString(x) }
func NewTerm(tilde bool, typ Type) *Term  { return impl.NewTerm(tilde, typ) }
func RelativeTo(pkg *Package) Qualifier   { return impl.RelativeTo(pkg) }
func IsSyncAtomicAlign64(t Type) bool     { return impl.IsSyncAtomicAlign64(t) }
